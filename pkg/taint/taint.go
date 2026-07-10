/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package taint

import (
	"context"
	"encoding/json"
	"math"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

// perAttemptTimeout bounds each individual Get/Patch round-trip so a single
// throttled request (apiserver Retry-After / HTTP 429) can never consume the
// whole removal budget the way a shared context would.
const perAttemptTimeout = 10 * time.Second

type jsonPatch struct {
	OP    string `json:"op,omitempty"`
	Path  string `json:"path,omitempty"`
	Value any    `json:"value"`
}

func remove(ctx context.Context, clientset *kubernetes.Clientset, node *corev1.Node, taintKey string) error {
	var taintsToKeep []corev1.Taint

	for _, taint := range node.Spec.Taints {
		if taint.Key != taintKey {
			taintsToKeep = append(taintsToKeep, taint)
		} else {
			klog.V(4).InfoS("Queued taint for removal", "key", taint.Key, "effect", taint.Effect)
		}
	}

	if len(taintsToKeep) == len(node.Spec.Taints) {
		klog.V(4).InfoS("No taints to remove, skipping taint removal", "node", node.Name)
		return nil
	}

	patchRemoveTaints := []jsonPatch{
		{OP: "test", Path: "/spec/taints", Value: node.Spec.Taints},
		{OP: "replace", Path: "/spec/taints", Value: taintsToKeep},
	}

	patch, err := json.Marshal(patchRemoveTaints)
	if err != nil {
		return err
	}

	// FieldManager attributes this change to kscu in the node's managedFields for auditability.
	_, err = clientset.CoreV1().Nodes().Patch(ctx, node.Name, k8stypes.JSONPatchType, patch, metav1.PatchOptions{FieldManager: "kscu"})
	if err != nil {
		return err
	}
	klog.V(4).InfoS("Removed taint from local node", "node", node.Name, "taint", taintKey)
	return nil
}

func hasTaint(n *corev1.Node, taintKey string) bool {
	for _, t := range n.Spec.Taints {
		if t.Key == taintKey {
			return true
		}
	}
	return false
}

// StartWatcher removes taintKey from the single node named nodeName.
//
// Removing a taint from one known node is a one-shot operation: Get the node,
// and if it still carries the taint, Patch it away. We deliberately do NOT use
// an informer/WatchList here - a heavyweight watch per node at boot is exactly
// the thundering-herd load that trips apiserver priority-and-fairness
// throttling, and a single throttled watch can stall long enough to leave the
// node permanently tainted (and therefore never Initialized under Karpenter).
//
// Instead we run a bounded Get->Patch retry loop. Each attempt gets its OWN
// fresh, short-lived context, so a throttled request fails one cheap attempt
// rather than poisoning the whole budget. maxWatchDuration is the overall
// wall-clock budget after which we give up (loudly).
func StartWatcher(clientset *kubernetes.Clientset, nodeName, taintKey string, maxWatchDuration time.Duration) {
	klog.V(2).InfoS("Starting taint removal loop", "node", nodeName, "taint", taintKey, "budget", maxWatchDuration.String())

	deadline := time.Now().Add(maxWatchDuration)
	backoff := wait.Backoff{
		Duration: 1 * time.Second,
		Factor:   1.5,
		Jitter:   0.2,
		Steps:    math.MaxInt32, // bounded by the wall-clock deadline below, not by step count
		Cap:      10 * time.Second,
	}

	for attempt := 1; ; attempt++ {
		removed, done := attemptTaintRemoval(clientset, nodeName, taintKey, attempt)
		if done {
			if removed {
				klog.V(2).InfoS("Taint removed", "node", nodeName, "taint", taintKey, "attempts", attempt)
			} else {
				klog.V(2).InfoS("Node has no taint, nothing to do", "node", nodeName, "taint", taintKey, "attempts", attempt)
			}
			return
		}

		delay := backoff.Step()
		if time.Now().Add(delay).After(deadline) {
			klog.ErrorS(nil, "Gave up removing taint within budget - NODE REMAINS TAINTED", "node", nodeName, "taint", taintKey, "budget", maxWatchDuration.String(), "attempts", attempt)
			return
		}
		klog.V(4).InfoS("Taint removal attempt failed, backing off", "node", nodeName, "attempt", attempt, "retryIn", delay.String())
		time.Sleep(delay)
	}
}

// attemptTaintRemoval performs one Get (+ optional Patch) round-trip on its own
// fresh context. It returns (removed, done):
//   - done=true  means no further work is needed (taint gone or successfully removed).
//   - done=false means this attempt failed and the caller should retry.
//   - removed indicates whether this run actually patched the taint away.
func attemptTaintRemoval(clientset *kubernetes.Clientset, nodeName, taintKey string, attempt int) (removed, done bool) {
	ctx, cancel := context.WithTimeout(context.Background(), perAttemptTimeout)
	defer cancel()

	node, err := clientset.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		klog.ErrorS(err, "Failed to get node", "node", nodeName, "attempt", attempt)
		return false, false
	}

	if !hasTaint(node, taintKey) {
		return false, true
	}

	klog.V(4).InfoS("Node has taint, removing", "node", nodeName, "taint", taintKey, "attempt", attempt)
	if err := remove(ctx, clientset, node, taintKey); err != nil {
		// A stale-node conflict (the JSON-Patch "test" op failed, or the node
		// changed under us) is expected and simply retried with a fresh Get.
		if apierrors.IsConflict(err) || apierrors.IsBadRequest(err) || apierrors.IsInvalid(err) {
			klog.V(4).InfoS("Node changed during patch, will re-get and retry", "node", nodeName, "attempt", attempt)
		} else {
			klog.ErrorS(err, "Failed to remove taint", "node", nodeName, "attempt", attempt)
		}
		return false, false
	}
	return true, true
}
