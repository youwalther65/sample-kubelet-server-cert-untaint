# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.0.0] - 2026-07-10

First tagged release. `kscu` is a Kubernetes DaemonSet that removes a startup
taint from an EKS node once the kubelet server certificate is present, closing
the race where pods are scheduled before the kubelet has its server cert and hit
TLS errors. Validated through customer testing on EKS (including EKS Auto Mode).

### Features

- Per-node taint removal driven by a short-lived, field-selector-scoped shared
  informer that watches only the local node.
- Taint removal via a JSON Patch with a `test` op for optimistic concurrency,
  with re-fetch and exponential backoff on stale-node conflicts, and a final
  "last chance" removal on timeout.
- Cert check that polls the local filesystem for
  `/var/lib/kubelet/pki/kubelet-server-current.pem` before removing the taint.
- `--skip-cert-check` mode to remove the taint immediately, usable as a generic
  node untainter unrelated to kubelet certs.
- Configurable timing via flags: `-taint-watcher-duration`, `-cert-check-duration`,
  `-cert-check-interval`, and `-startup-jitter` (boot-time jitter to spread
  DaemonSet load at scale).
- Configurable taint key (`-taint-key`) and required node name (`-node-name`,
  sourced from `NODE_NAME` via the downward API in the DaemonSet).
- Multi-arch container image (linux amd64 + arm64, Amazon Linux 2023 base) with
  a Trivy scan in the build, plus a Helm chart and raw Kubernetes manifests for
  deployment.
- Taint patch attributed to `kscu` via `FieldManager` for `managedFields`
  auditability.

[1.0.0]: https://github.com/aws-samples/sample-kubelet-server-cert-untaint/releases/tag/v1.0.0
