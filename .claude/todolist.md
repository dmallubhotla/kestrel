# Deploy routines — pluggable deploy spine: todolist

Goal (.claude/goal.md): scope the kubectl-manifest strategy vs a helm-chart
model, pick a design grounded in my repos' style, implement with smoke tests,
write docs + examples.

User steer (this session):
- Don't over-index on deploy-routines.md's "manifests win" lean.
- Leaning toward a common/3rd-party HELM CHART approach for proxmox-homelab,
  but BE FLEXIBLE: support local in-repo charts AND third-party/OCI charts.
- Build order: **helm first, manifest next.** Both on one shared spine.

## Design

`deploys:` map in .kestconfig (sibling of helm.releases), each entry is one app.
Executor chosen per entry by which source is set:
- `chart:` (+ optional repo/version/values/set/release_name) -> helm executor
- `manifests:` (dir or file)                                 -> kubectl executor
Shared: target->context/kubeconfig/profile resolution, guards, deploy scripts,
ambient-vs-profile policy, --diff/--dry-run. Cluster-agnostic (Talos named
context `admin@homelab`, optional explicit `kubeconfig` path on the target).

Existing `release` (helm.releases, OCI+image.tag+EKS) path stays untouched — it's
the work-repo single-service case. `deploys` is the general multi-app one.

## Steps — ALL DONE

- [x] 1. Design doc: appended "Decision & design approaches" to deploy-routines.md
        (manifest + helm-chart models, recommendation).
- [x] 2. Config: `deploys` map -> Deploy{...} + Kind()/Validate + target
        `kubeconfig`; compose + helpers. (config.go; tests pass.)
- [x] 3. internal/deploy: Resolution/Resolve/Policy/Execute + helm + kubectl
        executors + unit tests (deploy.go, executor.go, deploy_test.go).
- [x] 4. cmd/deploy.go (kest): deploy <app>|--all|--target|--diff|--force|-- args.
- [x] 5. cmd/kestci/deploy.go: ambient policy, guards always on.
- [x] 6. Smoke tests: test/deploy/deploy.sh (stub kubectl+helm, 23 assertions),
        wired as `deploy-smoke` flake check; test/deploy/RUNBOOK.md.
- [x] 7. Docs: docs/deploy.md + README usage + examples/deploy/ + roadmap update.
- [x] 8. go build / go test / smoke test / `nix flake check` all green.
        errcheck cleanup in printContext (deploy.go + sibling orchestrate.go);
        nix fmt cleared pre-existing yaml formatting.

Status: COMPLETE. Files staged (not committed — awaiting user). `.claude/` untracked.

## Runbooks
- test/deploy/RUNBOOK.md — manifest+helm smoke test + real-cluster procedure.
</content>
