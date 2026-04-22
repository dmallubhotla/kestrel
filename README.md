# kest

Kestrel (`kest`) is a CLI that wraps Helm and Terraform so you don't have to remember which AWS profile goes with which cluster, or what directory your terraform roots live in.
You set up a `.kestconfig` in your project and a global config on your machine, and kest figures out the rest.

It's particularly useful if you're deploying the same app to multiple EKS clusters across different AWS accounts, or managing a centralized IaC repo with dozens of terraform roots.

## What it does

- **Helm deploys** with automatic environment/values file layering, image tag resolution, and deploy scripts
- **Terraform orchestration** ("swoop") — discovers terraform roots in your repo, lets you pick them interactively or target them with globs, and tracks init/plan/apply state
- **Profile management** — set a target once, and all subsequent commands use it (no more `-e dev` on every invocation)
- **Safety guards** — CI-only enforcement, clean worktree checks, prod-only-from-main restrictions
- **`kest exec`** — run any command with the right AWS_PROFILE and kube context already set

## Install

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
accounts:
  "585912155334":
    aws_profile: dev-sso
  "593671994769":
    aws_profile: prd-sso

contexts:
  eks-dev: arn:aws:eks:us-east-1:585912155334:cluster/eks-dev
  eks-prd: arn:aws:eks:us-east-1:593671994769:cluster/eks-prd
  kind-local: kind-local
```

You can also set behavioral preferences:

```yaml
auto_sso_login: true        # auto aws sso login on expired sessions

swoop:
  auto_install_tf: true     # auto tfenv install on version mismatch (no prompt)
  cd_mode: pushd            # "cd" (default) or "pushd" for swoop cd
  editor: nvim              # override $EDITOR for swoop edit
  sort_order: recent        # "recent" (default) or "alpha"
```

All swoop settings are optional and have sensible defaults (cd, $EDITOR, no auto-install, recency-first ordering).

You can generate the accounts/contexts automatically with `kest config autoconfigure`, which scans your `~/.aws/config` and `~/.kube/config` and walks you through a TUI to pick what you want.

### Project config (`.kestconfig`)

This goes in your project root (committed to git). It defines your deployment targets and helm/terraform settings:

```yaml
helm:
  chart: oci://ghcr.io/myorg/mychart:1.0
  values_dir: misc/chart
  release_name: my-app
  namespace: app
  deploy_scripts:
    - misc/chart/deploy-scripts/migrate.sh

terraform:
  iac_dir: misc/iac

targets:
  dev:
    cluster: eks-dev
  prod:
    cluster: eks-prd
  local:
    cluster: kind-local
```

Kest walks up from your current directory to find this file, so it works from anywhere in your project tree.

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
kest -e dev release deploy
kest -e prod release deploy --force              # bypass all safety guards
kest release ls
kest release uninstall
```

Helm deploys layer values files (`shared.yaml` then `<target>.yaml`), resolve image tags (git tag for prod, `branch-sha` for everything else), and run any configured deploy scripts.

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

When you run `kest -e dev release deploy`, here's what happens:

1. Kest finds your `.kestconfig` and looks up the `dev` target → gets cluster name `eks-dev`
2. Looks up `eks-dev` in your global config's contexts → gets the full EKS ARN
3. Extracts the AWS account ID from the ARN → `585912155334`
4. Looks up that account ID in your global config's accounts → gets `aws_profile: dev-sso`
5. Runs helm with the right kube context and AWS_PROFILE set

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
