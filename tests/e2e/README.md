# MooseFS CSI Driver — End-to-End Test Runbook

This runbook covers the manual integration tests for the liveness /
mount-lifecycle hardening (issues #14, #30, #32; AGENTS.md items A–D).

## Prerequisites

- A local Kubernetes cluster reachable via `kubectl` (per AGENTS.md we
  test locally; e.g. kind/k3s/minikube with a node that can reach the
  MooseFS master).
- A MooseFS master reachable from the cluster nodes, with the
  `csi-moosefs-config` ConfigMap pointed at it
  (`master_host`, `master_port`, `mfs_mount_options` with
  `mfsmd5pass=...`).
- Docker, logged in to `hub.docker.com` for `make dev` pushes.

## Build & Deploy

```bash
# Build + push dev image (AMD64, dev tag) per AGENTS.md
make dev

# Deploy the driver
kubectl apply -f deploy/csi-moosefs-config.yaml
kubectl apply -f deploy/csi-moosefs.yaml

# Wait for the node DaemonSet to be ready on every node
kubectl -n kube-system rollout status daemonset/csi-moosefs-node --watch
```

## Test 1 — Dynamic provisioning baseline

```bash
kubectl apply -f examples/dynamic-provisioning/
kubectl wait --for=condition=Ready pod -l app=<example-app-label> --timeout=120s
kubectl get pvc
# Expect: Bound

# Write a sentinel file from the app pod
APP=$(kubectl get pod -l app=<example-app-label> -o jsonpath='{.items[0].metadata.name}')
kubectl exec "$APP" -- sh -c 'echo alive > /data/sentinel && sync'
kubectl exec "$APP" -- cat /data/sentinel
# Expect: alive
```

## Test 2 — Restart-survival (issue #32)

Verifies that published volumes stay usable across `csi-moosefs-node`
restarts (the core #32 fix: per-volume staging mounts in the host
namespace outlive the plugin container).

```bash
# Delete the node-plugin pod on the node running the app pod
NODE=$(kubectl get pod "$APP" -o jsonpath='{.spec.nodeName}')
kubectl -n kube-system delete pod -l app=csi-moosefs-node --field-selector spec.nodeName="$NODE"

# Wait for the plugin to come back
kubectl -n kube-system wait --for=condition=Ready pod -l app=csi-moosefs-node \
  --field-selector spec.nodeName="$NODE" --timeout=120s

# The app pod must STILL read its volume (no ENOTCONN, no remount)
kubectl exec "$APP" -- cat /data/sentinel
# Expect: alive   (no error, no "Transport endpoint is not connected")

# And still be writable
kubectl exec "$APP" -- sh -c 'echo still-alive >> /data/sentinel && cat /data/sentinel'
# Expect: alive \n still-alive
```

Pass criterion: app pod reads/writes succeed with **no** volume remount
and **no** ENOTCONN after the node plugin restarts.

## Test 3 — Stale-mount recovery (AGENTS.md item A)

Verifies that a stale FUSE mount (ENOTCONN) is detected, lazy-unmounted,
and re-mounted on the next stage/publish cycle.

```bash
# Find the staging mount path for the app's PVC on the host
STAGE=/var/lib/kubelet/plugins/kubernetes.io/csi/pv/<PV-NAME>/globalmount
# (inspect `kubectl get pv <PV-NAME> -o jsonpath='{.spec.csi.volumeAttributes}'` as needed)

# Force the mfsmount daemon for that staging mount to die, producing a
# stale (ENOTCONN) mount point on the host. From the node-plugin
# container (hostPID: true) find and kill the matching mfsmount PID:
kubectl -n kube-system exec -n <NODE> ds/csi-moosefs-plugin -- \
  sh -c 'pgrep -af mfsmount | grep <sub-path-token>'
# then kill -9 that PID via nsenter / host PID namespace.

# Confirm the staging path is now stale:
nsenter -t 1 -m -- stat "$STAGE"
# Expect: stat: ... Transport endpoint is not connected

# Trigger a re-stage by deleting + recreating the app pod
kubectl delete pod "$APP"
kubectl apply -f examples/dynamic-provisioning/<app>.yaml
kubectl wait --for=condition=Ready pod -l app=<example-app-label> --timeout=120s

# The new pod should mount cleanly and see prior data
kubectl exec "$NEWAPP" -- cat /data/sentinel
# Expect: alive \n still-alive
```

Pass criterion: no manual `umount -l` needed; the driver detects
ENOTCONN, lazy-unmounts, and remounts automatically. Check the
node-plugin logs for:

```
MountMfsVolumeStaged - staging target ... is stale (ENOTCONN/ESTALE), lazy-unmounting before remount
```

## Test 4 — Liveness probe triggers restart (AGENTS.md item D)

Verifies the `stat` liveness probe detects a dead pool mount and
kubelet restarts the container.

```bash
# Kill the pool-mount mfsmount from inside the plugin container.
# The pool mount path is /mnt/<NODE_ID>.
kubectl -n kube-system exec <node-plugin-pod> -- \
  sh -c 'pkill -f "mfsmount.*'"$(kubectl get node <NODE> -o jsonpath='{.metadata.name}')"'"'

# Watch the container restart
kubectl -n kube-system get pod <node-plugin-pod> -w
# Expect: RESTARTS increments within  failureThreshold * periodSeconds = 3 * 30s = 90s

# Confirm the container came back healthy and the pool mount is restored
kubectl -n kube-system exec <node-plugin-pod> -- stat /mnt/$(NODE_ID)
# Expect: exit 0
```

Pass criterion: container `RESTARTS` count increments within ~90s and
the pool mount is re-established after restart.

## Test 5 — Graceful shutdown cleans pool sessions (AGENTS.md item D)

Verifies that SIGTERM triggers pool-mount unmount (clean master session
close) while staged mounts persist.

```bash
# Baseline: count active sessions on the master for this node
# (e.g. via mfscli or the master admin UI) -> S0

# Delete the node-plugin pod (kubelet sends SIGTERM)
kubectl -n kube-system delete pod <node-plugin-pod> --grace-period=30

# After the pod terminates, count sessions again -> S1
# Expect: S1 < S0 (the pool session closed cleanly)

# Verify staged mounts are STILL present on the host (issue #32 contract)
nsenter -t 1 -m -- findmnt -M "$STAGE"
# Expect: still mounted (not torn down by graceful shutdown)
```

Pass criterion: master session count for the node drops by the number
of pool mounts, while staged volume mounts remain on the host.

## Test 6 — Node reboot (if a disposable dev node is available)

```bash
# Reboot the node running the app pod
ssh <node> sudo reboot

# After the node rejoins and the plugin reschedules:
kubectl -n kube-system wait --for=condition=Ready pod -l app=csi-moosefs-node \
  --field-selector spec.nodeName="$NODE" --timeout=300s

# The app pod should remount and see prior data
kubectl exec "$NEWAPP" -- cat /data/sentinel
# Expect: alive \n still-alive
```

Pass criterion: no leftover stale mount errors in the node-plugin logs
after reboot; app pod recovers its volume automatically.

## Unit tests (run before pushing the dev image)

```bash
go vet ./...
go test ./driver/... -count=1 -timeout 30s
```

All tests under `driver/` must pass, including the new ones:

- `TestMergeMountOptions*` — default options merge + user overrides
- `TestIsMountStale*` — ENOTCONN/ESTALE/healthy/other-errno classification
- `TestLazyUMountArgs*` — `umount -l` args + nsenter routing
- `TestServeWithGracefulShutdownOnSIGTERM` — gRPC GracefulStop on signal