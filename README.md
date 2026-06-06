# kest

Kestrel (`kest`) is a CLI that wraps Helm and Terraform so you don't have to remember which AWS profile goes with which cluster, or what directory your terraform roots live in.
You set up a `.kestconfig` in your project and a global config on your machine, and kest figures out the rest.

It's particularly useful if you're deploying the same app to multiple EKS clusters across different AWS accounts, or managing a centralized IaC repo with dozens of terraform roots.

## What it does

- **Helm deploys** with multi-release support, explicit values file layering, image tag resolution, and deploy scripts
- **Terraform orchestration** ("swoop") — discovers terraform roots in your repo, lets you pick them interactively or target them with globs, and tracks init/plan/apply state
- **Profile management** — set a target once, and all subsequent commands use it (no more `-e dev` on every invocation)
- **Safety guards** — CI-only enforcement, clean worktree checks, prod-only-from-main restrictions
- **`kest exec`** — run any command with the right AWS_PROFILE and kube context already set

## Install

Quick install (linux-amd64, linux-arm64, darwin-arm64):

```sh
curl -fsSL https://github.com/dmallubhotla/kestrel/releases/latest/download/install.sh | bash
```

See [`scripts/install.sh`](scripts/install.sh) or run it with `--help` for options.

Kest is built with Nix:

```sh
nix build    # produces ./result/bin/kest
```

Or if you just want to hack on it:

```sh
nix develop  # drops you into a shell with Go, terraform, helm, etc.
go build -o kest .
```

## Setup

Kest uses two config files.

### Global config (`~/.config/kest/config.yaml`)

This lives on your machine and maps AWS account IDs to profiles, and cluster names to kube contexts:

```yaml
aws:
  accounts:
    "111122223333":
      aws_profile: dev-sso
    "444455556666":
      aws_profile: prd-sso
  auto_sso_login: true          # auto aws sso login on expired sessions

kubernetes:
  contexts:
    eks-dev: arn:aws:eks:us-east-1:111122223333:cluster/eks-dev
    eks-prd: arn:aws:eks:us-east-1:444455556666:cluster/eks-prd
    kind-local: kind-local
```

You can also set behavioral preferences:

```yaml
terraform:
  command: tofu                 # CLI to invoke (default "terraform")
  version_manager: tofuenv      # "tfenv", "tofuenv", or "off" to disable
  auto_install_pinned: true     # auto-install pinned version on mismatch (no prompt)
  write_version: true           # write version-pin file if missing
  default_version: "1.9.2"     # version to pin (omit to detect from active terraform)

swoop:
  cd_mode: pushd                # "cd" (default) or "pushd" for swoop cd
  editor: nvim                  # override $EDITOR for swoop edit
  sort_order: recent            # "recent" (default) or "alpha"
```

All settings are optional and have sensible defaults.

You can generate the accounts/contexts automatically with `kest config autoconfigure`, which scans your `~/.aws/config` and `~/.kube/config` and walks you through a TUI to pick what you want.

### Project config (`.kestconfig`)

This goes in your project root (committed to git). It defines your deployment targets, helm releases, and terraform settings:

```yaml
helm:
  chart: oci://ghcr.io/myorg/mychart:1.0
  values_dir: misc/chart
  namespace: app
  deploy_scripts:
    - misc/chart/deploy-scripts/migrate.sh

  releases:
    other:
      release_name: my-app-other
      target: dev
      values:
        - dev.yaml
        - dev-other.yaml
    v1:
      release_name: my-app-v1
      target: local
      values:
        - local.yaml
      deploy_scripts: []      # skip deploy scripts for local

terraform:
  iac_dir: misc/iac

targets:
  dev:
    cluster: eks-dev
    aws_account: "111122223333"
    region: us-east-1
  prod:
    cluster: eks-prd
    aws_account: "444455556666"
    region: us-east-1
  local:
    cluster: kind-local
```

Each release specifies which target it deploys to. Values files are layered in order — `shared.yaml` (if it exists in `values_dir`) is always included first, then the files listed in `values`. Kest walks up from your current directory to find this file, so it works from anywhere in your project tree.

## Usage

### Profiles

Instead of passing `-e dev` every time:

```sh
kest profile use          # interactive picker
kest profile set dev      # non-interactive
kest profile current      # show what's active

# export to your shell
eval "$(kest profile export)"
```

### Helm

```sh
kest release deploy other                        # deploy a single release
kest release deploy --all                        # deploy all releases
kest release deploy --all --target dev           # deploy all dev releases
kest release deploy other       --force          # bypass all safety guards
kest release ls                                  # list configured releases
kest release ls other                            # query helm for release status
kest release uninstall other
```

Helm deploys layer `shared.yaml` (if present) then the release's explicit values files, resolve image tags (git tag for prod, `branch-sha` for everything else), and run any configured deploy scripts.

### Terraform

For simple project-embedded terraform:

```sh
kest -e dev terraform plan
kest -e dev terraform apply
```

### Swoop (terraform root discovery)

For repos with lots of terraform roots, swoop discovers them automatically by walking for `.tf` files with backend blocks:

```sh
kest swoop                      # interactive TUI picker
kest swoop list                 # list all roots
kest swoop list --dir prd       # filter by top-level directory
kest swoop plan "live/dev/*"    # glob targeting
kest swoop plan infra           # substring match
kest swoop plan --changed       # only roots with git changes
kest swoop edit dev/vpc         # open $EDITOR in root directory
eval "$(kest swoop cd dev/vpc)" # cd into root directory
```

Swoop tracks the last time you ran init/plan/apply on each root, so you can see at a glance what's stale. The TUI also supports `e` to edit and `c` to cd directly from the root picker.

### Running arbitrary commands

```sh
kest -e dev exec -- kubectl get pods
kest -e prod exec -- aws sts get-caller-identity
```

This sets up the right AWS_PROFILE and kube context before running your command.

### Config inspection

```sh
kest config paths       # where config files are (and if they loaded)
kest config show        # merged config as YAML
kest config targets     # list all targets with resolved context/profile
kest config accounts    # list account ID mappings
```

## Full config reference

Kest is opinionated: every field has a default. The blocks below show every
field with its default value plus realistic example entries for maps that
are inherently user-specific (AWS accounts, kube contexts, helm releases,
targets, directories). A repo or machine with no config file is treated as
if it had the scalar defaults below — the example map entries are
illustrative, not implicit.

### Terraform vs OpenTofu

By default kest invokes `terraform`. To use OpenTofu instead, set
`terraform.command: tofu` (in either the global config or `.kestconfig`)
or run with `KEST_TERRAFORM_COMMAND=tofu` for a one-off swap. Precedence
is `$KEST_TERRAFORM_COMMAND` → project `.kestconfig` → global config →
`"terraform"`.

The `version_manager` knob picks which companion CLI kest probes for
pinning workflows. Pick whichever your repo uses; kest itself stays binary-
agnostic:

- `tfenv` (default when command is `terraform`) — reads `.terraform-version`
- `tofuenv` (auto-default when command is `tofu`) — reads `.opentofu-version`
- `off` — disable kest's version-manager integration entirely: no PATH
  probe, no install offers, no mismatch warnings about a missing manager

When `write_version: true`, kest writes the file the resolved manager
reads — `.opentofu-version` for tofuenv, `.terraform-version` otherwise.
Discovery reads either file (`.opentofu-version` wins when both exist), so
a repo migrating between the two still recognises its pin during the
transition.

### Global config (`~/.config/kest/config.yaml`) — all fields with defaults

```yaml
# --- AWS ---
aws:
  # Map AWS account IDs to profile names from ~/.aws/config.
  # Inherently user-specific — the entries below are examples.
  accounts:
    "111122223333":
      aws_profile: dev-sso
    "444455556666":
      aws_profile: prd-sso
  # Automatically run `aws sso login` when a session is expired.
  # Skipped in CI. Default: false.
  auto_sso_login: false

# --- Kubernetes ---
kubernetes:
  # Map short cluster names to full kube context strings (typically EKS ARNs).
  # Inherently user-specific — the entries below are examples.
  contexts:
    eks-dev: arn:aws:eks:us-east-1:111122223333:cluster/eks-dev
    eks-prd: arn:aws:eks:us-east-1:444455556666:cluster/eks-prd
    kind-local: kind-local

# --- Terraform execution ---
terraform:
  # Terraform-compatible CLI to invoke. Set to "tofu" to use OpenTofu.
  # Overridden at runtime by $KEST_TERRAFORM_COMMAND. Default: "terraform".
  command: "terraform"
  # Version-manager CLI kest uses for version-pin handling.
  # "tfenv", "tofuenv", or "off". Empty auto-detects: "tofuenv" when the
  # resolved command is "tofu", else "tfenv". Overridden by
  # $KEST_TERRAFORM_VERSION_MANAGER. See "Terraform vs OpenTofu" above.
  version_manager: ""
  # Automatically install the pinned terraform version (from the
  # version-pin file) via the configured version_manager on mismatch,
  # without prompting. No-op when version_manager is "off". Skipped in CI.
  # Default: false.
  auto_install_pinned: false
  # Write a version-pin file (.opentofu-version when version_manager is
  # tofuenv, else .terraform-version) into roots that lack one before
  # running init/plan/apply. Default: false.
  write_version: false
  # Version to write when write_version is enabled. If empty, the
  # currently active terraform version is detected. Default: "" (detect).
  default_version: ""

# --- Swoop (interactive terraform root browser) ---
swoop:
  # Shell command emitted by `swoop cd`: "cd" or "pushd". Default: "cd".
  cd_mode: cd
  # Editor for `swoop edit`. Empty means use $EDITOR. Default: "".
  editor: ""
  # Root ordering in the TUI: "git" (dirty-first + recency), "recent",
  # or "alpha". Default: "git".
  sort_order: git
```

### Project config (`.kestconfig`) — all fields with defaults

```yaml
# --- Helm ---
helm:
  # OCI chart reference. Required for helm deploys; defaults to "".
  chart: oci://ghcr.io/myorg/mychart:1.0
  # Directory containing values files. shared.yaml in this dir is
  # auto-included if it exists; all other values are listed per release.
  values_dir: misc/chart
  # Kubernetes namespace. Default: "app".
  namespace: app
  # Scripts to run before each helm deploy (paths relative to project root).
  # Can be overridden per release (set deploy_scripts: [] to skip).
  deploy_scripts:
    - misc/chart/deploy-scripts/migrate.sh
  # Named helm releases. Inherently project-specific — the entries below
  # are examples showing the field shape. Each release targets exactly one
  # entry in `targets:` below.
  releases:
    other:
      release_name: my-app-other     # helm release name passed to `helm upgrade`
      target: dev                    # must match a key in targets:
      values:                        # values files (relative to values_dir)
        - dev.yaml
        - dev-other.yaml
    v1:
      release_name: my-app-v1
      target: local
      values:
        - local.yaml
      deploy_scripts: []             # override top-level scripts; [] = skip

# --- Terraform ---
terraform:
  # Path to IaC directory (swoop discovery base for centralised IaC repos).
  # Default: "" (project root).
  iac_dir: misc/iac
  # command can also be set here to pin the CLI per-project (e.g. "tofu").
  # Project overrides global. $KEST_TERRAFORM_COMMAND overrides both.
  # Default: "".
  command: ""
  # version_manager can also be set here to pin the manager per-project.
  # Project overrides global. $KEST_TERRAFORM_VERSION_MANAGER overrides
  # both. Default: "".
  version_manager: ""
  # default_version can also be set here to pin a project-wide terraform
  # version for write_version. Default: "".
  default_version: ""

# --- Targets ---
# Named deployment targets binding a cluster + AWS account + region. Helm
# releases reference these by key; swoop also resolves through them.
# Inherently project-specific — the entries below are examples.
targets:
  dev:
    cluster: eks-dev               # must resolve via kubernetes.contexts above
    aws_account: "111122223333"    # must resolve via aws.accounts above
    region: us-east-1
  prod:
    cluster: eks-prd
    aws_account: "444455556666"
    region: us-east-1
  local:
    cluster: kind-local            # cluster-only targets are valid (no AWS)

# --- Directories (swoop only) ---
# For centralised-IaC repos: map a top-level directory name to an AWS
# account ID, so swoop can resolve credentials by path even when a root
# has no provider/account in its .tf files. Inherently project-specific.
directories:
  prd: "444455556666"
  dev: "111122223333"
```

## How resolution works

When you run `kest release deploy other`, here's what happens:

1. Kest finds your `.kestconfig` and looks up the `other` release → gets target `dev`
2. Looks up the `dev` target → gets cluster name `eks-dev`, account `111122223333`
3. Looks up `eks-dev` in your global config's contexts → gets the full EKS ARN
4. Looks up that account ID in your global config's accounts → gets `aws_profile: dev-sso`
5. Runs helm with `shared.yaml` + the release's values files, the right kube context, and AWS_PROFILE set

For swoop, resolution works through directory→account ID mappings instead, but the account→profile lookup is the same.

## Development

```sh
just build    # nix build
just test     # run all tests
just check    # nix flake check
just fmt      # format nix files
just chores   # go mod tidy + regenerate gomod2nix.toml
```

Run a specific test:

```sh
go test ./internal/config/ -run TestName
```

After changing `go.mod`, always run `just chores` to keep `gomod2nix.toml` in sync.

## Project structure

```
cmd/           CLI layer (Cobra commands)
internal/
  config/      Two-layer config loading and target resolution
  profile/     Active profile persistence
  helm/        Helm deploy command construction
  terraform/   Terraform command proxying
  guard/       Deploy safety checks
  runner/      External command execution
  swoop/       Terraform root discovery, resolution, execution, state tracking
  logging/     Structured JSON logging
  awsconfig/   AWS config file parsing
  kubeconfig/  Kube config file parsing
```
