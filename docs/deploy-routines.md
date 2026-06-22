# Deploy routines (helm + manifests) — design exploration

Status: **exploratory, not decided.** Material for a future design discussion,
grounded in how proxmox-homelab actually deploys today. The point is to let the
real usage shape the design rather than fit usage to the existing helm path.

## What we're designing for

The homelab has two distinct deploy shapes, and kest's current helm path
matches neither:

1. **Platform helm charts** — terraform `helm_release` (traefik, cert-manager,
   longhorn, metallb, kube-prometheus, loki, …) in `iac-modules/k8s-platform`.
   `repository` + `chart` + `version` + inline `set{}`, kubeconfig via
   `config_path` to the Talos cluster's kubeconfig. Applied by
   `kest swoop apply k8s-platform`, tracked in terraform state.
2. **Apps** — raw manifests at `k8s-manifests/<app>/NN-*.yaml`, deployed by
   `kubectl apply -f k8s-manifests/<app>/` (documented in the README, run by
   hand). No context resolution, no guards, no record.

kest's current helm shape — OCI chart ref + layered values files +
`aws eks update-kubeconfig` (`internal/helm`, `kestci release deploy`) — is
unused here: wrong cluster type (Talos ≠ EKS), wrong artifact (no OCI charts),
and the apps aren't helm at all.

**So the gap kest would actually fill here is not "OCI helm → EKS." It's "apply
a chart/manifest to the right cluster with resolved context, the guard stack,
and a record."** The most underserved piece is the raw-manifest app deploy.

## Option A — make the existing helm path cluster-agnostic

Generalize the kubeconfig source so a target resolves its context from EKS
(today), a kubeconfig path/terraform output (Talos), or a named context.

```yaml
# .kestconfig
targets:
  homelab:
    cluster: admin@homelab
    kubeconfig: iac-live/talos-cluster/kubeconfig   # NEW: explicit source, not EKS
```

```sh
kest release deploy <release> --target homelab       # existing flow, Talos kubeconfig
```

Verdict: necessary if non-EKS helm is ever wanted, but **low value here** — the
homelab has no OCI charts/values layout, and its helm already lives in
terraform (where state tracking is fine). Don't move it out just to use this.

## Option B — a manifest-apply primitive (new shape)

The real gap. A verb that applies a manifest directory to a resolved target,
reusing target/context resolution + guards + exec-log + the spine's
ambient/profile policy:

```sh
kest deploy authentik --target homelab     # kubectl apply -f k8s-manifests/authentik/ on Talos ctx
kest deploy --all --target homelab
kestci deploy authentik                    # CI: ambient kubeconfig, identical resolution
```

```yaml
# .kestconfig — PROPOSED, sibling of helm.releases
deploys:
  authentik:
    manifests: k8s-manifests/authentik
    target: homelab
  firefly:
    manifests: k8s-manifests/firefly-iii
    target: homelab
```

Reuses: target→context resolution, guard stack, exec-log, ambient-vs-profile
policy. New: a `kubectl apply` executor. Ordering falls out for free — the
`NN-` prefixes sort lexically, which is how `kubectl apply -f <dir>` already
orders. Open questions: a plan analog (`kubectl diff`); whether to ever support
`--prune` (dangerous, probably not); namespaces (manifests carry their own).

## Option C — leave helm in terraform; kest owns only the app gap

Pragmatic split: don't touch the terraform-helm platform charts. Add Option B
for apps. Helm-via-kest stays the EKS/work-repo feature where it fits; the
Talos homelab uses terraform-helm + kest manifest-deploy.

## The shape this points at

Letting usage lead: the homelab wants **Option B**, and Option A only matters
when a work repo wants non-EKS helm. The existing `internal/helm` functions are
reusable for the *resolution + guard + record* scaffolding; only the executor
differs (`helm upgrade` vs `kubectl apply`).

That's the same move as the terraform spine we just built: **one
resolve → execute → record spine with a pluggable executor** —
`helm_upgrade | kubectl_apply | (even) tf_helm` — behind one resolution and
policy layer shared by `kest` and `kestci`. Worth weighing whether the deploy
side should be built on that spine from the start rather than growing a second
forked path the way terraform did.

## Questions to settle in the discussion

- Is the homelab's need really Option B (manifests), with helm-to-Talos (A) a
  separate, later, work-repo-driven concern? (Leaning yes.)
- Should `deploy` get a plan/dry-run analog (`kubectl diff`) to mirror
  `swoop plan`, so PR-plan CI works for apps too?
- Do we build the deploy side on a shared spine immediately, given we just
  learned what forking costs on the terraform side?
