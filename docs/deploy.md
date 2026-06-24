# `kest deploy` — app deploys (helm charts + raw manifests)

`kest deploy` applies an **app** to a resolved cluster, picking its executor
automatically: a helm chart (`helm upgrade --install`) or a directory of raw
manifests (`kubectl apply -f`). It's the cluster-agnostic, multi-app path —
Talos, kind, bare named contexts, EKS — with the same target resolution, guard
stack, and ambient-vs-profile policy that `kest swoop` uses for terraform.

It is the non-EKS sibling of [`kest release`](../README.md#helm). Use `release`
for the single-OCI-chart, env-layered, image-tag-resolving EKS work-repo flow;
use `deploy` for "a bunch of apps, each its own chart or manifest set, on
whatever cluster." For the design rationale see
[deploy-routines.md](deploy-routines.md).

## Config

Apps go under `deploys:` in `.kestconfig` (sibling of `helm.releases`). Each
entry sets **exactly one** source — `chart:` (helm) or `manifests:` (kubectl):

```yaml
deploys:
  # 1. local in-repo chart (a shared chart, or per-app — your call)
  homepage:
    chart: charts/app                 # local path, oci:// ref, or repo chart name
    values: [deploys/homepage.yaml]   # values files, layered in order (project-root relative)
    namespace: homepage               # optional; adds --create-namespace
    target: homelab

  # 2. third-party chart from a repo
  authentik:
    chart: authentik/authentik
    repo: https://charts.goauthentik.io
    version: "2024.10.1"
    values: [deploys/authentik.yaml]
    set:                              # optional inline --set overrides
      authentik.error_reporting.enabled: "false"
    target: homelab

  # 3. raw manifests (applied in NN- filename order, like kubectl apply -f dir/)
  gitea:
    manifests: k8s-manifests/gitea
    target: homelab

targets:
  homelab:
    cluster: admin@homelab            # a named kube context (Talos, kind, …),
                                      # resolved via kubernetes.contexts or used
                                      # literally — not necessarily an EKS ARN
    kubeconfig: iac-live/talos-config/kubeconfig   # optional explicit kubeconfig
```

Fields, per deploy:

| Field | Executor | Meaning |
| --- | --- | --- |
| `chart` | helm | local path, `oci://…` ref, or chart name (with `repo`) |
| `repo` | helm | chart repository URL (`--repo`) |
| `version` | helm | chart version (`--version`) |
| `release_name` | helm | helm release name (default: the deploy's key) |
| `values` | helm | values files, project-root relative, layered in order |
| `set` | helm | inline `--set key=value` overrides |
| `manifests` | kubectl | directory or file applied with `kubectl apply -f` |
| `namespace` | both | `--namespace` (+ `--create-namespace` for helm) |
| `target` | both | **required** — key in `targets:` |
| `deploy_scripts` | both | override `helm.deploy_scripts` (`nil` = inherit, `[]` = skip) |

### Target resolution (cluster-agnostic)

A target's `cluster` is looked up in your global `kubernetes.contexts`; if it
isn't mapped, the literal value is used as the context name. So a Talos context
like `admin@homelab` "just works" once it's in your kubeconfig (e.g. merged via
`just kubeconfig`). The optional `kubeconfig:` points at an explicit file — handy
in CI, or to read a terraform-output kubeconfig without merging it into
`~/.kube/config`. AWS profile resolution is unchanged and stays empty for
non-AWS clusters.

## Usage

```sh
kest deploy gitea                    # kubectl apply -f k8s-manifests/gitea/
kest deploy homepage                 # helm upgrade --install homepage charts/app …
kest deploy homepage --diff          # read-only preview: helm --dry-run / kubectl diff
kest deploy --all                    # every app
kest deploy --all --target homelab   # every app on a target
kest deploy gitea -- --server-side   # pass extra args straight to kubectl/helm
kest deploy gitea --force            # bypass guards (CI-only, clean-worktree, branch)
```

`--diff` is the plan analog: `helm upgrade --dry-run` for charts, `kubectl diff`
for manifests (a present diff exits non-zero — kest treats that as "there's a
diff", not a failure). It's read-only and skips the guard stack.

### Guards

Applies go through the same guards as `kest release`: CI-only, clean worktree,
and prod-only-from-main. `--force` bypasses all three for local applies.
`--diff` is exempt (read-only).

## In CI (`kestci deploy`)

`kestci deploy <app>` is the non-interactive sibling: identical resolution, but
**ambient** credentials (it never sets `AWS_PROFILE`; kubeconfig comes from the
environment) and guards are always enforced — there is no `--force`.

```sh
kestci deploy authentik              # apply, ambient creds, clean-worktree + branch guards
kestci deploy --all --target homelab
kestci deploy gitea --diff           # read-only
```

Supply the kubeconfig the way that fits your runner: a `KUBECONFIG` secret, an
explicit `kubeconfig:` on the target (e.g. a terraform-output file written by an
earlier job step), or — for EKS — an `aws eks update-kubeconfig` step before
`kestci deploy`. See [ci.md](ci.md) for the blast-radius tiering that decides
*which* clusters should be driven from CI at all (Tier-2 platform/SSO stacks
usually shouldn't).

## What `deploy` is not

- **Not `kest release`.** No `image.tag` resolution, no `shared.yaml`/`<target>`
  values layering, no `aws eks update-kubeconfig`. That's the EKS work-repo path.
- **Not GitOps.** No `--prune`, no drift reconcile — `--diff` is the only read
  path. Argo/Flux own continuous reconciliation if you want it.
- **Not the platform.** Cluster-tier helm (traefik, cert-manager, longhorn, …)
  belongs in terraform `helm_release` where its state lives. `deploys:` is for
  apps running *on* the platform.
