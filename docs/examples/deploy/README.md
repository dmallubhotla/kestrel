# Example: `kest deploy` combining terraform + helm + manifests

A worked layout for a non-EKS cluster (kind/k3s, or any named context) where:

- **terraform** stands up the cluster and platform, and emits a kubeconfig;
- **`kest deploy`** rolls out apps on top — some as helm charts (a shared
  in-repo chart and a third-party chart), some as raw manifests.

```
examples/deploy/
├── .kestconfig                 # deploys: + targets: (this is the whole wiring)
├── charts/app/                 # one shared chart; homemade apps are values of it
│   ├── Chart.yaml
│   ├── values.yaml
│   └── templates/{deployment,service}.yaml
├── deploys/homepage.yaml       # per-app values for the shared chart
└── k8s-manifests/hello/        # a raw-manifest app (NN- ordered)
    ├── 00-namespace.yaml
    └── 10-deployment.yaml
```

## How terraform and kest hand off

terraform owns the cluster + platform and writes a kubeconfig as an output; the
target's `kubeconfig:` points at that file, so kest deploys against exactly the
cluster terraform built — no manual `~/.kube/config` merge required:

```hcl
# iac-live/cluster/outputs.tf (in your infra repo)
output "kubeconfig" { value = module.cluster.kubeconfig, sensitive = true }
```

```sh
# 1. infra: cluster + platform helm (traefik, longhorn, …) live here, in TF state
kest swoop apply cluster
kest swoop apply k8s-platform
tofu -chdir=iac-live/cluster output -raw kubeconfig > iac-live/cluster/kubeconfig

# 2. apps: kest deploy takes over from here
kest deploy --all --target homelab
```

The split is deliberate: **platform helm stays in terraform** (its state belongs
there); **app helm + manifests move to `kest deploy`**. See
[../../docs/deploy-routines.md](../../docs/deploy-routines.md).

## Try it on kind

```sh
kind create cluster --name kest-example
# point the target at the kind context (edit .kestconfig: cluster: kind-kest-example,
# and drop the kubeconfig: line to use your ~/.kube/config)

cd examples/deploy
kest deploy hello --diff           # preview the manifest app
kest deploy hello --force          # apply it (--force: local, not a clean repo)
kest deploy homepage --force       # the shared-chart app
kubectl --context kind-kest-example get pods -A | grep -E 'hello|homepage'

kind delete cluster --name kest-example
```

In CI the same config runs as `kestci deploy --all --target homelab` with
ambient credentials and guards always on.
