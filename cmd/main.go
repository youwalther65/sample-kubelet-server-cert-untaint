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

package main

import (
	"context"
	"os"
	"time"

	"k8s.io/klog/v2"

	"github.com/aws-samples/sample-kubelet-server-cert-untaint/pkg/certcheck"
	"github.com/aws-samples/sample-kubelet-server-cert-untaint/pkg/config"
	"github.com/aws-samples/sample-kubelet-server-cert-untaint/pkg/k8sclient"
	"github.com/aws-samples/sample-kubelet-server-cert-untaint/pkg/taint"
	"github.com/aws-samples/sample-kubelet-server-cert-untaint/pkg/util"
)

func main() {
	klog.InitFlags(nil)
	util.SetupSignalHandler()
	cfg := config.Load()

	clientSet, err := k8sclient.NewClientset(cfg.KubeconfigPath)
	if err != nil {
		klog.ErrorS(err, "Failed to create clientset from the given config")
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), config.KubeletServerCertCheckDuration)
	defer cancel()

	if cfg.SkipCertCheck {
		klog.V(2).InfoS("Skipping cert check, removing taint immediately", "node", cfg.NodeName, "taint", cfg.TaintKey)
		taint.StartWatcher(clientSet, cfg.NodeName, cfg.TaintKey, config.TaintWatcherDuration)
	} else if !certcheck.WaitForCert(ctx, cfg.CheckInterval) {
		klog.ErrorS(nil, "Still no kubelet server certificate after timeout", "timeout", config.KubeletServerCertCheckDuration)
	} else {
		klog.V(2).InfoS("Running taint removal now", "node", cfg.NodeName, "taint", cfg.TaintKey)
		taint.StartWatcher(clientSet, cfg.NodeName, cfg.TaintKey, config.TaintWatcherDuration)
	}

	klog.V(2).InfoS("No more work to done - sleep forever now")
	klog.Flush()

	const maxDuration time.Duration = 1<<63 - 1
	time.Sleep(maxDuration)
}
