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

package certcheck

import (
	"context"
	"time"

	"github.com/aws-samples/sample-kubelet-server-cert-untaint/pkg/config"
	"github.com/aws-samples/sample-kubelet-server-cert-untaint/pkg/util"

	"k8s.io/klog/v2"
)

func WaitForCert(ctx context.Context, checkInterval time.Duration) bool {
	start := time.Now()
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()
	klog.V(2).InfoS("Check kubelet server certificate", "interval", checkInterval)
	var attempts int
	for {
		attempts++
		if util.FileExists(config.DefaultKubeletCertFile) {
			klog.V(4).InfoS("kubelet server certificate exists", "check", attempts, "elapsed", time.Since(start).String())
			return true
		}
		klog.V(4).InfoS("Still no kubelet server certificate - re-trying", "check", attempts, "elapsed", time.Since(start).String())
		// Wait one interval before the next check, but honor the overall
		// deadline promptly instead of blocking on a context-blind sleep.
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
		}
	}
}
