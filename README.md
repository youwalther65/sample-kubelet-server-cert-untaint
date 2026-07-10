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
certCheckInterval: 10

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
- `certCheckInterval`: Interval in seconds to check for the certificate (default: `5`)
- `startupJitter`: Max random delay in seconds before each pod contacts the apiserver, spreading DaemonSet boot load at scale to avoid throttling; `0` disables (default: `3`)
- `taintWatcherDuration`: Overall wall-clock budget in seconds for removing the taint before giving up (default: `30`)
- `certCheckDuration`: Max time in seconds to wait for the kubelet server certificate to appear (default: `75`)
- `logLevel`: Verbosity level for logging (default: `4`)
- `skipCertCheck`: Skip kubelet certificate check (default: false)

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
- `cert-check-interval`: Interval in seconds to check for the certificate (default: `5`)
- `startup-jitter`: Max random delay in seconds before contacting the apiserver, spreading DaemonSet boot load at scale to avoid throttling; `0` disables (default: `3`)
- `taint-watcher-duration`: Overall wall-clock budget in seconds for removing the taint before giving up (default: `30`)
- `cert-check-duration`: Max time in seconds to wait for the kubelet server certificate to appear (default: `75`)
- `log-level`: Verbosity level for logging (default: `4`)
- `skip-cert-check`: Skip kubelet certificate check (default: false)

All settings are command-line flags. The one exception is the node name, which is read from the `NODE_NAME` environment variable: in the DaemonSet it is injected from `spec.nodeName` via the downward API, which can only populate an env var, not a flag.

Note: With `skip-cert-check` command line parameter set or Helm parameter `skipCertCheck` set to `true`, `kscu` will skip the certificate check and immediately untaints node. That makes it possible to use `kscu` as a generic untainter. 

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
$ kubectl logs -n kube-system kubelet-server-cert-untaint-<redacted>
I0707 09:30:17.157817       1 config.go:71] "Starting taint remover" Version="1.1.0" GitCommit="0346747c0b84f314ccc0e7042a7b3e9852825e96" BuildDate="2026-07-07T08:58:05+00:00"
I0707 09:30:17.157906       1 config.go:88] "Configuration" node="i-<redacted>" taint="example.com/kubelet-no-server-cert" cert-check-interval=5 startup-jitter=3 skip-cert-check=false
I0707 09:30:17.752291       1 util.go:45] "Applying startup jitter before contacting apiserver" delay="976.633554ms" max="3s"
I0707 09:30:18.729447       1 envvar.go:172] "Feature gate default state" feature="InOrderInformersBatchProcess" enabled=true
I0707 09:30:18.729481       1 envvar.go:172] "Feature gate default state" feature="InformerResourceVersion" enabled=true
I0707 09:30:18.729487       1 envvar.go:172] "Feature gate default state" feature="WatchListClient" enabled=true
I0707 09:30:18.729491       1 envvar.go:172] "Feature gate default state" feature="ClientsAllowCBOR" enabled=false
I0707 09:30:18.729495       1 envvar.go:172] "Feature gate default state" feature="ClientsPreferCBOR" enabled=false
I0707 09:30:18.729499       1 envvar.go:172] "Feature gate default state" feature="InOrderInformers" enabled=true
I0707 09:30:18.729641       1 client.go:39] "Connected to K8s cluster"
I0707 09:30:18.729671       1 certcheck.go:33] "Check kubelet server certificate" interval="5s"
I0707 09:30:19.729844       1 certcheck.go:42] "kubelet server certificate exists" check=1 elapsed="1.00016154s"
I0707 09:30:19.729871       1 main.go:57] "Running taint removal now" node="i-<redacted>" taint="example.com/kubelet-no-server-cert"
I0707 09:30:19.729881       1 taint.go:102] "Starting taint removal loop" node="i-<redacted>" taint="example.com/kubelet-no-server-cert" budget="30s"
I0707 09:30:19.740562       1 taint.go:153] "Node has taint, removing" node="i-<redacted>" taint="example.com/kubelet-no-server-cert" attempt=1
I0707 09:30:19.740783       1 taint.go:52] "Queued taint for removal" key="example.com/kubelet-no-server-cert" effect="NoExecute"
I0707 09:30:19.773332       1 taint.go:75] "Removed taint from local node" node="i-<redacted>" taint="example.com/kubelet-no-server-cert"
I0707 09:30:19.773362       1 taint.go:117] "Taint removed" node="i-<redacted>" taint="example.com/kubelet-no-server-cert" attempts=1
I0707 09:30:19.773370       1 main.go:61] "No more work to be done - sleeping forever now"
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

## Disclaimer

This is sample code provided for educational and demonstration purposes only. See [DISCLAIMER.md](DISCLAIMER.md) — use at your own risk; AWS is not responsible for any damages, data loss, or costs arising from its use.

## License

Licensed under the Apache License, Version 2.0.
