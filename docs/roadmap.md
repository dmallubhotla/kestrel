# Roadmap / backlog

Ideas captured but not scoped for the next release. Order is rough priority, not commitment.

## Per-guard config flags (replace blanket `--force`)

Today `--force` (`cmd/root.go`) bypasses all three guards in `internal/guard/guard.go` at once: `CheckCI`, `CheckCleanWorktree`, `CheckBranch`. That's coarse — quick local applies from a laptop only really need the CI guard off, not the dirty-worktree or branch ones.

Proposed shape (global config only, never project config — must not be committable):

```yaml
guards:
  require_ci: false              # default true
  require_clean_worktree: true   # default true
  require_main_for_prod: true    # default true
```

Notes:
- Each guard consults config before erroring; `--force` stays as a master override on the CLI.
- `kest doctor` should surface a loud warning whenever any of these are disabled, so a misconfigured machine is visible.
- Project-level `.kestconfig` must not be allowed to flip these — otherwise a repo could ship "no CI required" to everyone.

## OpenTofu binary override

Right now `"terraform"` is hardcoded in five places: `internal/terraform/terraform.go`, `internal/swoop/executor.go` (×3), `cmd/doctor.go`, plus the version probe in `cmd/config_autoconfigure.go`.

Proposed shape:

```yaml
terraform:
  binary: tofu     # default "terraform"
```

Plus a `$KEST_TERRAFORM_BIN` env override for one-off swaps.

Scope notes:
- `tfenv` stays a **per-user switch** (independent of binary choice). It's a tfenv-only tool; OpenTofu users wanting auto-install need `tofuenv`, which is a separate decision per user — don't auto-branch it on binary.
- Version output parsing in `parseTFVersionOutput` needs to also accept `OpenTofu v...`.
- `kest doctor` should probe whichever binary is configured.
