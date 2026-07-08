# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

A Kubernetes DaemonSet (Go binary named `kscu`) that removes a startup taint from an EKS node once the kubelet server certificate (`/var/lib/kubelet/pki/kubelet-server-current.pem`) is present. This closes a race where pods get scheduled before the kubelet has its server cert, causing TLS errors (e.g. for GitLab runner executor pods, or `kubectl exec/logs`). The taint is applied at node startup (typically via a Karpenter NodePool `startupTaints`), and `kscu` removes it per-node.

## Architecture

The binary runs once per node and does a single job, then sleeps forever (a DaemonSet pod must stay alive). Flow in `cmd/main.go`:

1. `config.Load()` — parse flags/env, resolve node name (required).
2. `k8sclient.NewClientset()` — in-cluster config by default; falls back to `KUBECONFIG` / `~/.kube/config` when running locally.
3. Either skip the cert check (`--skip-cert-check`) or `certcheck.WaitForCert()` — polls for the cert file up to `KubeletServerCertCheckDuration` (75s).
4. `taint.StartWatcher()` — removes the taint, then sleeps forever.

Package responsibilities (`pkg/`):
- **config** — flags, env vars, and timing config. `TaintWatcherDuration` (default 30s, `-taint-watcher-duration`) and `KubeletServerCertCheckDuration` (default 75s, `-cert-check-duration`) are configurable flags in seconds; `taintremoverVersion`/`gitCommit`/`buildDate` are injected at build time via `-ldflags`.
- **certcheck** — polls the local filesystem for the cert file. Purely local; no API calls.
- **taint** — the core logic. Uses a short-lived, field-selector-scoped **shared informer** (watching only this node) plus event handlers to remove the taint. Removal uses a **JSON Patch with a `test` op** on `/spec/taints` for optimistic concurrency; on `test` failure / stale-node errors it re-fetches the node and retries with exponential backoff. A `sync.Mutex.TryLock` prevents overlapping removal attempts. On timeout it does one final "last chance" `Get` + removal. Watch permission errors (`Unauthorized`/`Forbidden`) silently cancel rather than spamming logs.
- **k8sclient** — clientset construction.
- **util** — file-existence check and SIGTERM/SIGINT graceful-shutdown handler.

Logging is klog structured logging throughout; verbosity is controlled by `-v` (default level 4 in deployments).

### The `--skip-cert-check` mode
With this flag (Helm: `skipCertCheck`), `kscu` skips the cert wait and removes the taint immediately, making it usable as a generic node untainter unrelated to kubelet certs.

## Templating: the critical gotcha

Several deployment files are **generated from `.template` files by `make envsubst`** and are gitignored — do NOT edit the generated files directly, edit the `.template` and re-run:
- `deploy/kubernetes/daemonset.yaml` ← `daemonset.yaml.template`
- `charts/.../values.yaml` ← `values.yaml.template`
- `charts/.../Chart.yaml` ← `Chart.yaml.template`

`make envsubst` also rewrites the `module` line in `go.mod` and can rename the chart directory to match `$(NAME)`. It substitutes `NAME`, `VERSION`, `IMAGE_REGISTRY`, `IMAGE_REPOSITORY`, `TAG`, `PKG`. Nearly every deploy/helm Make target depends on `envsubst`.

`VERSION` (Makefile) must match the chart version in `Chart.yaml` — the Makefile comments call this out.

## Common commands

```bash
make build            # mod-tidy + fmt + vet + build static binary to bin/kscu
make run              # run locally: go run ./cmd/main.go -node-name <NODE_NAME> -v 4
make vet / make fmt   # go vet / go fmt
make clean            # remove bin/ and build/

make docker-build     # multi-arch (linux amd64+arm64, al2023) build + push + trivy scan; requires IMAGE_REGISTRY
make install-kscu      # envsubst + kubectl apply rbac.yaml and daemonset.yaml
make install-kscu-helm # envsubst + helm upgrade --install
make install-sample    # deploy sample NodePool (auto-detects EKS Auto Mode) + sample Deployment
make helm-lint / helm-template / helm-package / helm-push
make help             # full target list
```

There is **no test suite** in this repo (no `*_test.go` files). `make build` runs `go vet` as the primary static check.

## Configuration surface

Flags (and env equivalents) — see `pkg/config/config.go`:
- `-taint-key` / `TAINT_KEY` — taint to remove (default `example.com/kubelet-no-server-cert`).
- `-node-name` / `NODE_NAME` — **required**; in the DaemonSet it comes from `spec.nodeName` via the downward API.
- `-cert-check-interval` — cert poll interval seconds (default 5).
- `-startup-jitter` — max random boot delay seconds before contacting the apiserver (default 3, 0 disables).
- `-taint-watcher-duration` — overall taint-removal wall-clock budget seconds (default 30).
- `-cert-check-duration` — max cert-wait seconds (default 75).
- `-skip-cert-check` — remove taint immediately.
- `-kubeconfig` — for local runs.

The taint key in the DaemonSet/Helm values **must match** the `startupTaints` key in the Karpenter NodePool, and the DaemonSet `nodeAffinity` must select the same nodes — otherwise Karpenter never considers the node `Initialized` and won't consolidate it.

## Build/runtime notes

- Go module path is `github.com/aws-samples/sample-kubelet-server-cert-untaint`; Go 1.26.
- Dockerfile is multi-stage: golang builder → `amazonlinux:2023` minimal runtime; entrypoint `/bin/app`.
- The DaemonSet mounts `/var/lib/kubelet/pki` read-only, runs privileged with `readOnlyRootFilesystem`, `system-node-critical` priority, and broad tolerations so it schedules onto tainted nodes.
- RBAC needs `nodes`: `get, patch, list, watch` (ClusterRole).
