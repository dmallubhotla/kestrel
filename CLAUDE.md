# CLAUDE.md

Guidance for working in this repo. `kest` wraps helm and terraform per-project;
build/test with the `justfile` (`just check`) or `go test ./...`.

## This repo is public

`kestrel` is developed against the author's real home cluster, but the repo is
public. Never commit real details about that infrastructure — not in code,
comments, docs, examples, or tests. The specific cluster OS/distro, the
hypervisor, real AWS account IDs, and real kubeconfig or repo paths are all
off-limits, even though they're what we run against day to day.

Use generic placeholders that illustrate the feature without revealing the
real setup:

- Kube contexts: `my-cluster`, `eks-dev` — not real context names.
- AWS account IDs: `111122223333`, `444455556666` — never a real 12-digit id.
- Cluster type: "a named context", "kind / k3s", "EKS" — not a specific distro.
- Paths: `iac-live/cluster/kubeconfig` and similar neutral examples.

## Comments

Keep a comment only if it says something the code doesn't — intent, a
constraint, or a non-obvious "why". Default to cutting.

- **No redundancy.** Don't restate the code in prose. `args = append(args,
  "--dry-run")` does not need `// add --dry-run`.
- **No narration of the edit.** Comments describe the code as it is, not what
  you just changed or how it differs from before ("now uses…", "switched
  to…", "removed the old…" — all out).
- **Stay relevant — this is also how leaks start.** A comment should mention
  only what bears on the code it documents. Incidental detail about the dev
  environment is both noise and a privacy leak: most personal-info leaks in
  this repo began as an over-explaining comment that named a specific
  cluster/distro it never needed to mention. A properly scoped comment would
  never have contained that detail in the first place.
- **Terse and factual.** Minimal, no editorializing asides.

Good: `// kubectl diff exits 1 when differences exist; higher codes are real errors.`

Bad: `// We run kubectl diff here, added for the Talos homelab; it exits 1 when there's a diff so we treat that as success.`
