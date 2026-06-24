- explicitly authorized to edit this file to better reflect goal scope and details

- use a todolist file for different steps, and runbooks for any tests you perform

- [x] scope out the kubectl manifest strategy  
  - I would consider testing migrating to a hel mchatr model for my services, i'd store charts here and use kest to handle the helm usage.
    I can see pluses and minuses to that approach, but really helm isn't _that_ much better.
    with terraform creating secrets and generic deploy scripts for stuff there's some scope for that to be good.
    But still scope out using k8s manifests and implement (include smoke test scripts).
  - DONE: scoped in docs/deploy-routines.md ("Decision & design approaches"):
    manifest models (thin kubectl-apply / kustomize / GitOps) + helm-chart models
    (one shared in-repo chart / per-app charts / third-party). Per session steer,
    built the helm path first, manifests next; flexible chart source.
- [x] For both k8s manifest and helm chart, come up with a few software fesign approaches, research my own projects dir for repos to see my style if you need to resolve preference questions tetc.
  - DONE: researched proxmox-homelab (NN- manifest dirs, Talos admin@homelab,
    platform helm in terraform). Approaches written up in deploy-routines.md.
- [x] implement yoru recommended design, and then create docs and example uses (potentially combining terraform + helm)
  - DONE: `kest deploy` / `kestci deploy` on a shared pluggable-executor spine
    (internal/deploy). Config: deploys: map + target kubeconfig:. Smoke tests
    (test/deploy/deploy.sh, wired as the deploy-smoke flake check) + unit tests.
    Docs: docs/deploy.md, README usage, examples/deploy/ (terraform+helm+manifests).
    `nix flake check` (just check) passes: formatting, golangci-lint,
    terraform-compat, deploy-smoke all green.
