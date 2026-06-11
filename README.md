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

Kest uses two config files: a global one on your machine and a project one (`.kestconfig`) committed to your repo. Every field has a default; the blocks below show every field with its default value. A repo or machine with no config file is treated as if it had exactly the defaults below.

### Global config (`~/.config/kest/config.yaml`)

```yaml
# --- AWS ---
aws:
  # Map AWS account IDs to profile names from ~/.aws/config. Example:
  #   accounts:
  #     "111122223333":
  #       aws_profile: dev-sso
  #     "444455556666":
  #       aws_profile: prd-sso
  accounts: {}
  # Automatically run `aws sso login` when a session is expired.
  # Skipped in CI.
  auto_sso_login: false

# --- Kubernetes ---
kubernetes:
  # Map short cluster names to full kube context strings (typically EKS ARNs).
  # Example:
  #   contexts:
  #     eks-dev: arn:aws:eks:us-east-1:111122223333:cluster/eks-dev
  #     kind-local: kind-local
  contexts: {}

# --- Terraform execution ---
terraform:
  # Terraform-compatible CLI to invoke. Set to "tofu" to use OpenTofu.
  # Overridden at runtime by $KEST_TERRAFORM_COMMAND.
  command: "terraform"
  # Version-manager CLI kest uses for version-pin handling.
  # "tfenv", "tofuenv", or "off". Empty auto-detects: "tofuenv" when the
  # resolved command is "tofu", else "tfenv". Overridden by
  # $KEST_TERRAFORM_VERSION_MANAGER. See "Terraform vs OpenTofu" below.
  version_manager: ""
  # Automatically install the pinned terraform version (from the
  # version-pin file) via the configured version_manager on mismatch,
  # without prompting. No-op when version_manager is "off". Skipped in CI.
  auto_install_pinned: false
  # Write a version-pin file (.opentofu-version when version_manager is
  # tofuenv, else .terraform-version) into roots that lack one before
  # running init/plan/apply.
  write_version: false
  # Version to write when write_version is enabled. Empty means detect the
  # currently active terraform version at write time.
  default_version: ""

# --- Swoop (interactive terraform root browser) ---
swoop:
  # Shell command emitted by `swoop cd`: "cd" or "pushd".
  cd_mode: cd
  # Editor for `swoop edit`. Empty means use $EDITOR.
  editor: ""
  # Root ordering in the TUI: "git" (dirty-first + recency), "recent",
  # or "alpha".
  sort_order: git
```

You can generate the accounts/contexts automatically with `kest config autoconfigure`, which scans your `~/.aws/config` and `~/.kube/config` and walks you through a TUI to pick what you want.

### Project config (`.kestconfig`)

Goes in your project root (committed to git). Kest walks up from your current directory to find it, so it works from anywhere in your project tree.

```yaml
# --- Helm ---
helm:
  # OCI chart reference. Required for helm deploys.
  # Example: oci://ghcr.io/myorg/mychart:1.0
  chart: ""
  # Directory containing values files (listed per release under values:).
  # Example: misc/chart
  values_dir: ""
  # Kubernetes namespace.
  namespace: app
  # Scripts to run before each helm deploy (paths relative to project root).
  # Can be overridden per release (set deploy_scripts: [] on a release to skip).
  # Example:
  #   deploy_scripts:
  #     - misc/chart/deploy-scripts/migrate.sh
  deploy_scripts: []
  # Named helm releases. Each release targets exactly one entry in
  # `targets:` below. Example:
  #   releases:
  #     other:
  #       release_name: my-app-other   # passed to `helm upgrade`
  #       target: dev                  # must match a key in targets:
  #       values:                      # values files (relative to values_dir)
  #         - dev.yaml
  #         - dev-other.yaml
  #     v1:
  #       release_name: my-app-v1
  #       target: local
  #       values: [local.yaml]
  #       deploy_scripts: []           # override top-level scripts; [] = skip
  releases: {}

# --- Terraform ---
terraform:
  # Path to IaC directory (swoop discovery base for centralised IaC repos).
  # Empty means the project root. Example: misc/iac
  iac_dir: ""
  # command can also be set here to pin the CLI per-project (e.g. "tofu").
  # Project overrides global. $KEST_TERRAFORM_COMMAND overrides both.
  command: ""
  # version_manager can also be set here to pin the manager per-project.
  # Project overrides global. $KEST_TERRAFORM_VERSION_MANAGER overrides both.
  version_manager: ""
  # default_version can also be set here to pin a project-wide terraform
  # version for write_version.
  default_version: ""

# --- Targets ---
# Named deployment targets binding a cluster + AWS account + region. Helm
# releases reference these by key; swoop also resolves through them. Example:
#   targets:
#     dev:
#       cluster: eks-dev             # must resolve via kubernetes.contexts
#       aws_account: "111122223333"  # must resolve via aws.accounts
#       region: us-east-1
#     local:
#       cluster: kind-local          # cluster-only targets are valid (no AWS)
targets: {}

# --- Directories (swoop only) ---
# For centralised-IaC repos: map a top-level directory name to an AWS
# account ID, so swoop can resolve credentials by path even when a root
# has no provider/account in its .tf files. Example:
#   directories:
#     prd: "444455556666"
#     dev: "111122223333"
directories: {}
```

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

Helm deploys layer the release's values files in order, resolve image tags (git tag for prod, `branch-sha` for everything else), and run any configured deploy scripts.

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

## How resolution works

When you run `kest release deploy other`, here's what happens:

1. Kest finds your `.kestconfig` and looks up the `other` release → gets target `dev`
2. Looks up the `dev` target → gets cluster name `eks-dev`, account `111122223333`
3. Looks up `eks-dev` in your global config's contexts → gets the full EKS ARN
4. Looks up that account ID in your global config's accounts → gets `aws_profile: dev-sso`
5. Runs helm with the release's values files, the right kube context, and AWS_PROFILE set

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
