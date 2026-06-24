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

---

# Decision & design approaches

Status: **decided, implemented.** This section supersedes the "leaning Option B"
exploration above. The earlier text assumed the homelab's app gap was
*manifests*; revisiting it, the homelab is at least as likely to move its apps to
a **helm chart** model (a shared in-repo chart, per-app charts, or third-party
charts). So the design can't pick manifests *or* helm — it has to make the deploy
**verb** indifferent to which one a given app uses.

## What we're actually choosing between

The question isn't "manifests vs helm" at the tool level — it's **how many forked
deploy paths kest grows.** Two axes:

1. **Artifact** — raw manifests (`kubectl apply -f dir/`) vs a helm chart
   (`helm upgrade --install`). An app picks one; the *repo* will have both.
2. **Chart source**, if helm — a single shared in-repo chart (every app is a
   release of `charts/app/` differing only by values), per-app in-repo charts, or
   third-party/OCI charts. We don't need to pick one: the config takes a `chart:`
   that is a local path, an `oci://` ref, or a `repo` + chart name.

### Manifest-model approaches

- **A1 — thin `kubectl apply -f` primitive** *(chosen for the manifest side).*
  Apply a directory/file to a resolved context. Ordering falls out of the `NN-`
  filename convention kubectl already sorts on. Plan analog = `kubectl diff`.
  Zero new concepts; matches exactly what the homelab does by hand today.
- **A2 — kustomize-aware** (`kubectl apply -k`). Strictly more powerful, but the
  homelab manifests aren't kustomized; adding it now is speculative. `-- -k` style
  passthrough leaves the door open without a config surface.
- **A3 — full GitOps reconcile** (track applied set, prune removed objects).
  Rejected: `--prune` is foot-gun-shaped, and a reconcile loop is Argo/Flux's job,
  not a CLI verb's.

### Helm-chart-model approaches

- **B1 — single shared in-repo chart.** Most reuse: one `charts/app/`, every app a
  values file. Best when apps are homogeneous (Deployment + Service +
  IngressRoute + PVC). Escape hatch needed for the odd app that doesn't fit.
- **B2 — per-app in-repo charts.** Most boilerplate, most independence. Each app
  diverges freely; no shared-template blast radius.
- **B3 — third-party / OCI charts.** No chart authoring at all — point at upstream
  (`oci://…`, or a `repo` URL + chart name + version) and supply values.

The homelab will likely **mix B1 and B3** (shared chart for the homemade apps,
upstream charts for things like authentik). So the tool must not force a choice:
`chart:` accepts a local path *or* an OCI ref *or* a repo chart, and the same
`deploys:` entry shape covers all three.

## The chosen shape: one deploy verb, pluggable executor

A new `deploys:` map in `.kestconfig`, sibling to `helm.releases`. **Each entry is
one app, and which executor runs is inferred from which source field it sets:**

```yaml
deploys:
  homepage:                      # shared in-repo chart (B1)
    chart: charts/app
    values: [deploys/homepage.yaml]
    target: homelab
  authentik:                     # third-party chart (B3)
    chart: authentik/authentik
    repo: https://charts.goauthentik.io
    version: "2024.10.1"
    values: [deploys/authentik.yaml]
    target: homelab
  gitea:                         # raw manifests (A1)
    manifests: k8s-manifests/gitea
    target: homelab

targets:
  homelab:
    cluster: admin@homelab       # a named context, resolved via kubernetes.contexts
                                 # (or used literally) — not an EKS ARN
    kubeconfig: iac-live/talos-config/kubeconfig   # optional explicit source
```

```sh
kest deploy homepage              # helm upgrade --install against the Talos context
kest deploy gitea                 # kubectl apply -f k8s-manifests/gitea/
kest deploy gitea --diff          # kubectl diff (plan analog); helm gets --dry-run
kest deploy --all --target homelab
kestci deploy authentik           # identical resolution, ambient credentials
```

This is the same move as the terraform spine: **one resolve → execute → record
loop with a pluggable executor** (`helm` | `kubectl`), parameterized by a small
`Policy` (ambient-vs-profile, dry-run) shared by `kest` and `kestci`. Adding a new
artifact type later (e.g. `kustomize`) is a new executor, not a new command.

### What this deliberately is *not*

- **Not a replacement for `kest release` / `helm.releases`.** That path stays the
  work-repo case: one OCI chart, env-layered values (`shared.yaml` → `<target>.yaml`
  → release values), `image.tag` resolution (git tag / `branch-sha`), EKS
  kubeconfig via `aws eks update-kubeconfig`. `deploys:` is the general, multi-app,
  cluster-agnostic path; it does **no** image-tag magic and does **not** assume EKS.
- **Not GitOps.** No prune, no drift reconcile. `--diff` is the only read path.
- **Not owning the platform helm.** Cluster-tier helm (traefik, cert-manager,
  longhorn, metallb, …) stays in terraform `helm_release` where its state belongs
  (the old Option C). `deploys:` is for *apps*, not the platform under them.

### Cluster-agnostic resolution

`deploys` resolves a target to a kube **context** (and optional explicit
`kubeconfig` path), not an EKS ARN. A target's `cluster` is looked up in
`kubernetes.contexts`; if unmapped, the literal value is used as the context name
(so Talos's `admin@homelab` just works once merged into `~/.kube/config`). An
optional `kubeconfig:` on the target points at a file (e.g. a terraform output),
which matters in CI where `~/.kube/config` isn't pre-populated. AWS profile
resolution is unchanged and simply stays empty for non-AWS clusters.

### Why not just generalize `helm.releases`?

`helm.releases` is built around *one* `cfg.Helm.Chart` shared by all releases and
a forced `--set image.tag=`. The homelab has *many different* charts (and raw
manifests), no shared chart, and apps whose images aren't kest's to tag. Bending
the single-chart schema to cover per-app charts + manifests would overload it past
recognition. A sibling `deploys:` keeps each path honest.
