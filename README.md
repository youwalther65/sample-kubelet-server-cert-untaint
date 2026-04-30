# Kubelet Server Certificate Untaint

This project is a reference implementation of a Kubernetes DaemonSet that automatically removes a taint from nodes once the kubelet server certificate is available.

## Background

When Kubernetes nodes start up, there can be a race condition where pods are scheduled before the kubelet gets a server certificate. This can cause issues for some workloads like GitLab runner, resulting in TLS errors for GitLab worker/executor pods.

This tool solves the problem by:
1. Waiting for the kubelet server certificate to exist (`/var/lib/kubelet/pki/kubelet-server-current.pem`)
2. Removing a configurable taint from the node once the certificate is present
3. Allowing safe pod scheduling only after the kubelet is fully ready

Users can apply a taint (e.g., `example.com/kubelet-no-server-cert:NoSchedule`) to nodes at startup, and this DaemonSet will automatically remove it once the kubelet certificate is available.

### kubelet internals

After kubelet start up it creates a K8s CertificateSigningRequest which is eventually approved and signed by an EKS control plane component called `eks-certificate-controller`. This can take up to 60 seconds. The signed certificate is then stored in node OS under `/var/lib/kubelet/pki/kubelet-server-current.pem` which is a soft-link to a time-stamped PEM file in the same directory.
For commands like `kubectl exec/logs` kube-apiserver initiates a connection to the kubelet via TLS which requires this kubelet server certificate.

In EKS Auto Mode Container Network Interface (CNI) is running as a systemd service and there is no need to start an aws-node pod. containerd can notify kubelet about CRI and CNI `Ready` almost immediately, causing kubelet posting `Ready` status very fast, approximately within 2s after start up, almost at same time when kubelet creates the CertificateSigningRequest.  
Pods can then scheduled to the node and start running. This increases the likelihood that a missing kubelet server certificate causes TLS issues.

In none EKS Auto Mode it takes about 20..25s for kubelet to get `Ready`. Usually the kubelet server already certificate is available  at that point in time.

## Installation

### Create a Karpenter NodePool or Managed Node Group which applies a startUp taint
It is recommended to install the DaemonSet to the required subset of nodes by creating a a Karpenter NodePool or an EKS Managed Node Group, which applies the taint to the nodes. For Karpenter one can use the attribute `startupTaints`.

The folder `deploy/karpenter` provides a demo NodePool `sample` for standard EKS and EKS Auto Mode.
For EKS Auto Mode use:
```yaml
kubectl apply -f deploy/karpenter/sample-nodepool-auto-mode.yaml
```
which applies a startupTaint key `example.com/kubelet-no-server-cert` with action `NoExecute`.

Note: Karpenter treats [startupTaints](https://karpenter.sh/docs/concepts/nodepools/#spectemplatespecstartuptaints) in such a way, that they need to be removed before the nodeclaim is considered `Initialized` and pods can be deployed to a node. If the startup taint is not removed for whatever reason, the nodeclaim and corresponding node will not be consolidated or deleted by Karpenter. Therefore it is important that `startupTaints` in the NodePool matches the configuration in the DaemonSet `kubelet-server-cert-untaint` (`nodeAffinity` and `taint-key`).

### Using Helm

1. Configure your values in `custom-values.yaml` according to your needs:  
The `taintKey` needs to match the `startupTaints` from the Karpenter NodePool. The `nodeAffinity` applies the DaemonSet only to nodes managed by the Karpenter NodePool `sample`.

```yaml
taintKey: "example.org/kubelet-no-server-cert"
checkInterval: 10

nodeAffinity:
  requiredDuringSchedulingIgnoredDuringExecution:
    nodeSelectorTerms:
      - matchExpressions:
          - key: kubernetes.io/os
            operator: In
            values:
              - linux
          - key: karpenter.sh/nodepool
            operator: In
            values:
              - sample
```

2. Install the chart:
```bash
helm install sample-kubelet-server-cert-untaint \
  charts/sample-kubelet-server-cert-untaint \
  -f charts/sample-kubelet-server-cert-untaint/custom-values.yaml \
  -n kube-system
```

#### Configuration

Key Helm parameters:
- `taintKey`: The taint key to watch for and remove (default: `example.com/kubelet-no-server-cert`)
- `checkInterval`: Interval in seconds to check for the certificate (default: `5`)
- `logLevel`: Verbosity level for logging (default: `4`)
- `skipCertCheck`: Skip kubelet certificate check (default: false)

Note: With `skipCertCheck` set to `true`, kscu will skip the certificate check and immediately untaints node. That makes it possible to use kscu as a generic untainter. 

### Using kubectl

Apply the Kubernetes manifests directly:
```bash
kubectl apply -f deploy/kubernetes/rbac.yaml
kubectl apply -f deploy/kubernetes/daemonset.yaml
```
The manifests work together with a NodePool `sample` from above.

#### Configuration

Key `kscu` command line parameters:
- `taint-key"`: The taint key to watch for and remove (default: `example.com/kubelet-no-server-cert`)
- `check-interval`: Interval in seconds to check for the certificate (default: `5`)
- `log-level`: Verbosity level for logging (default: `4`)

## Testing

The folder `deploy/karpenter` provides a demo deployment `sample-deploy` with a `nodeSelector` to run pods on nodes provisioned by NodePool `sample`.

To install use:
```yaml
kubectl apply -f deploy/karpenter/sample-deploy.yaml
```
To start the workload scale the deployment:
```yaml
kubectl scale deploy sample-deploy --replicas 1
```
and check the logs of the DaemonSet pod `kubelet-no-server-cert` created on the node provisiond by Karpenter.

It should look like:
```bash
$ kubectl logs -n kube-system kubelet-server-cert-untaint-<redacted> -f
I0302 16:14:28.696802       1 config.go:50] "Starting taint remover" Version="1.1" GitCommit="034db65d82226d06125cecb536fd84fe4edb186d" BuildDate="2026-03-02T14:25:30+00:00"
I0302 16:14:28.696893       1 config.go:63] "Configuration" node="i-<redacted>" taint="example.com/kubelet-no-server-cert" check-interval=5
I0302 16:14:28.697078       1 envvar.go:172] "Feature gate default state" feature="InOrderInformers" enabled=true
I0302 16:14:28.697089       1 envvar.go:172] "Feature gate default state" feature="InOrderInformersBatchProcess" enabled=true
I0302 16:14:28.697092       1 envvar.go:172] "Feature gate default state" feature="InformerResourceVersion" enabled=true
I0302 16:14:28.697095       1 envvar.go:172] "Feature gate default state" feature="WatchListClient" enabled=true
I0302 16:14:28.697098       1 envvar.go:172] "Feature gate default state" feature="ClientsAllowCBOR" enabled=false
I0302 16:14:28.697100       1 envvar.go:172] "Feature gate default state" feature="ClientsPreferCBOR" enabled=false
I0302 16:14:28.697303       1 client.go:23] "Connected to K8s cluster"
I0302 16:14:28.697324       1 certcheck.go:17] "Check kubelet server certificate" interval="5s"
I0302 16:14:29.697523       1 certcheck.go:29] "Still no kubelet server certificate - re-trying" check=1 elapsed="1.000185211s"
I0302 16:14:34.697938       1 certcheck.go:29] "Still no kubelet server certificate - re-trying" check=2 elapsed="6.000600196s"
I0302 16:14:39.698145       1 certcheck.go:26] "kubelet server certificate exists" check=3 elapsed="11.000806321s"
I0302 16:14:39.698187       1 main.go:50] "Running taint removal now" node="i-<redacted>" taint="example.com/kubelet-no-server-cert"
I0302 16:14:39.698196       1 taint.go:70] "startNotReadyTaintWatcher - creating short-lived node informer" node="i-<redacted>" maxWatchDuration="30s"
I0302 16:14:39.698456       1 reflector.go:367] "Starting reflector" type="*v1.Node" resyncPeriod="5s" reflector="/gomodcache/k8s.io/client-go@v0.35.1/tools/cache/reflector.go:289"
I0302 16:14:39.698468       1 reflector.go:411] "Listing and watching" type="*v1.Node" reflector="/gomodcache/k8s.io/client-go@v0.35.1/tools/cache/reflector.go:289"
I0302 16:14:39.797317       1 reflector.go:978] "Exiting watch because received the bookmark that marks the end of initial events stream" reflector="/gomodcache/k8s.io/client-go@v0.35.1/tools/cache/reflector.go:289" totalItems=2 duration="98.815644ms"
I0302 16:14:39.797353       1 reflector.go:446] "Caches populated" type="*v1.Node" reflector="/gomodcache/k8s.io/client-go@v0.35.1/tools/cache/reflector.go:289"
I0302 16:14:39.797416       1 taint.go:91] "Node has taint, remove taint" node="i-<redacted>" taint="example.com/kubelet-no-server-cert"
I0302 16:14:39.797458       1 taint.go:33] "Queued taint for removal" key="example.com/kubelet-no-server-cert" effect="NoExecute"
I0302 16:14:39.798934       1 taint.go:91] "Node has taint, remove taint" node="i-<redacted>" taint="example.com/kubelet-no-server-cert"
I0302 16:14:39.813660       1 taint.go:56] "Removed taint from local node" node="i-<redacted>" taint="example.com/kubelet-no-server-cert"
I0302 16:14:39.814432       1 taint.go:87] "Node has no taint, do nothing" node="i-<redacted>" taint="example.com/kubelet-no-server-cert"
I0302 16:14:39.844419       1 taint.go:87] "Node has no taint, do nothing" node="i-<redacted>" taint="example.com/kubelet-no-server-cert"
I0302 16:14:44.798265       1 reflector.go:466] "Forcing resync" reflector="/gomodcache/k8s.io/client-go@v0.35.1/tools/cache/reflector.go:289"
I0302 16:14:44.798345       1 taint.go:87] "Node has no taint, do nothing" node="i-<redacted>" taint="example.com/kubelet-no-server-cert"
I0302 16:14:49.265739       1 taint.go:87] "Node has no taint, do nothing" node="i-<redacted>" taint="example.com/kubelet-no-server-cert"
I0302 16:14:49.798903       1 reflector.go:466] "Forcing resync" reflector="/gomodcache/k8s.io/client-go@v0.35.1/tools/cache/reflector.go:289"
I0302 16:14:49.798990       1 taint.go:87] "Node has no taint, do nothing" node="i-<redacted>" taint="example.com/kubelet-no-server-cert"
I0302 16:14:54.803081       1 reflector.go:466] "Forcing resync" reflector="/gomodcache/k8s.io/client-go@v0.35.1/tools/cache/reflector.go:289"
I0302 16:14:54.803145       1 taint.go:87] "Node has no taint, do nothing" node="i-<redacted>" taint="example.com/kubelet-no-server-cert"
I0302 16:14:59.805007       1 reflector.go:466] "Forcing resync" reflector="/gomodcache/k8s.io/client-go@v0.35.1/tools/cache/reflector.go:289"
I0302 16:14:59.805089       1 taint.go:87] "Node has no taint, do nothing" node="i-<redacted>" taint="example.com/kubelet-no-server-cert"
I0302 16:15:04.807665       1 reflector.go:466] "Forcing resync" reflector="/gomodcache/k8s.io/client-go@v0.35.1/tools/cache/reflector.go:289"
I0302 16:15:04.807733       1 taint.go:87] "Node has no taint, do nothing" node="i-<redacted>" taint="example.com/kubelet-no-server-cert"
I0302 16:15:09.826720       1 reflector.go:466] "Forcing resync" reflector="/gomodcache/k8s.io/client-go@v0.35.1/tools/cache/reflector.go:289"
I0302 16:15:09.826789       1 taint.go:87] "Node has no taint, do nothing" node="i-<redacted>" taint="example.com/kubelet-no-server-cert"
I0302 16:15:09.926473       1 taint.go:87] "Node has no taint, do nothing" node="i-<redacted>" taint="example.com/kubelet-no-server-cert"
I0302 16:15:09.926511       1 main.go:54] "No more work to done - sleep forever now"
I0302 16:15:09.926584       1 reflector.go:373] "Stopping reflector" type="*v1.Node" resyncPeriod="5s" reflector="/gomodcache/k8s.io/client-go@v0.35.1/tools/cache/reflector.go:289"
```

## Building

Build the binary:
```bash
make build
```

Multi-platform build of container images and manifest file:
```bash
make docker-build
```

## License

Licensed under the Apache License, Version 2.0.
