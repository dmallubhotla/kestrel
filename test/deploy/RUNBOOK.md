# Runbook — `kest deploy` testing

Two layers cover the deploy spine. Run the first on every change; run the second
when you touch resolution or executor flags and want real-cluster confidence.

## 1. Hermetic smoke test (no cluster)

`test/deploy/deploy.sh` builds `kest`/`kestci`, puts **stub** `kubectl` and
`helm` on `PATH` that echo their argv, and asserts the right command is built
from a real `.kestconfig` for each executor (manifest + helm), `--diff`,
third-party charts, `--all`, passthrough, explicit kubeconfig, and the guard
posture.

```sh
# builds the binaries itself:
test/deploy/deploy.sh

# or against an existing binary:
KEST=./result/bin/kest KESTCI=./result/bin/kestci test/deploy/deploy.sh
```

Also runs in the Nix sandbox as the `deploy-smoke` flake check (`just check` /
`nix flake check`). Requires only bash + a Go toolchain (or prebuilt binaries);
no kubectl, helm, or cluster.

Unit tests in `internal/deploy` cover argument construction in isolation:

```sh
go test ./internal/deploy/
```

## 2. Real-cluster test (manual)

Exercises the path against an actual cluster — a local `kind`/`k3d`, or the
homelab cluster. Use a throwaway namespace.

### Setup

```sh
# kind is easiest for a disposable target:
kind create cluster --name kest-deploy-test
kubectl config get-contexts          # note the context, e.g. kind-kest-deploy-test
```

Create a scratch project:

```sh
mkdir -p /tmp/kest-deploy/k8s-manifests/hello
cat > /tmp/kest-deploy/k8s-manifests/hello/00-ns.yaml <<'YAML'
apiVersion: v1
kind: Namespace
metadata: { name: kest-hello }
YAML
cat > /tmp/kest-deploy/k8s-manifests/hello/10-deploy.yaml <<'YAML'
apiVersion: apps/v1
kind: Deployment
metadata: { name: hello, namespace: kest-hello }
spec:
  replicas: 1
  selector: { matchLabels: { app: hello } }
  template:
    metadata: { labels: { app: hello } }
    spec:
      containers:
        - { name: hello, image: nginx:alpine }
YAML
cat > /tmp/kest-deploy/.kestconfig <<'YAML'
deploys:
  hello:
    manifests: k8s-manifests/hello
    target: local
targets:
  local:
    cluster: kind-kest-deploy-test    # your context name (used literally)
YAML
```

### Run

```sh
cd /tmp/kest-deploy
kest deploy hello --diff               # read-only: shows the create diff (exit 1 = diff present)
kest deploy hello --force              # apply (--force: not in CI, scratch dir not a clean repo)
kubectl --context kind-kest-deploy-test -n kest-hello get pods   # expect hello-* Running
kest deploy hello --diff               # now clean: "no changes" / empty diff
```

For the **helm** path, add a chart (e.g. `helm create charts/app`) and a
`deploys:` entry with `chart: charts/app`; `kest deploy <app> --diff` should
print the rendered manifests (`--dry-run`) and `--force` should `helm upgrade
--install` it. Verify with `helm --kube-context <ctx> -n <ns> list`.

### Homelab cluster variant

Same, but the target's `cluster` is a bare named context like `my-cluster`
(merge it into `~/.kube/config` first, e.g. via `just kubeconfig` in the
homelab repo), or set an explicit `kubeconfig:` path on the target pointing at
the terraform-output kubeconfig.

### Teardown

```sh
kubectl --context kind-kest-deploy-test delete ns kest-hello
kind delete cluster --name kest-deploy-test
rm -rf /tmp/kest-deploy
```

## Results log

| Date | Layer | Binary / cluster | Result |
| --- | --- | --- | --- |
| 2026-06-21 | hermetic smoke (`deploy.sh`) | `go build` HEAD | 23 passed, 0 failed |
| 2026-06-21 | unit (`go test ./internal/deploy/`) | HEAD | ok |
| 2026-06-21 | `nix flake check` (incl. `deploy-smoke` sandbox check) | HEAD | exit 0 (all 4 checks green) |
