# Running kest from the container image

Every release publishes `ghcr.io/dmallubhotla/kestrel` (linux amd64/arm64), tagged `X.Y.Z`, `X.Y`, `X`, and `latest`. It bundles `kest` and `kestci` plus every tool they shell out to, so nothing needs to be installed in the container:

| Tool | Used for |
| --- | --- |
| `terraform` | swoop / terraform commands (version manager is disabled — the image pins this terraform) |
| `tofu` | OpenTofu alternative — used instead of terraform when your repo sets `terraform.command: tofu` (or `KEST_TERRAFORM_COMMAND=tofu`) |
| `helm` | release deploys |
| `kubectl` | context switching (`kest exec`, `kest profile`, `kest context`) |
| `aws` | SSO login, session checks, `eks update-kubeconfig` (kestci), and the `aws eks get-token` exec plugin your kubeconfig uses for every EKS API call |
| `git` | clean-worktree / branch guards, image-tag resolution from git describe |
| `gh` | GitHub credential helper for git operations against private repos |
| `bash` | deploy scripts |

The container runs as `kest` (uid 1000) with home `/home/kest` and working directory `/work`. Since `HOME` is env-set rather than uid-derived, any `--user` override works too (`/home/kest` is writable by all uids):

- **Rootful docker** with a uid-1000 host user (the common single-user linux case): everything just works, and files written to your mounted repo are owned by you. Other uids: add `--user "$(id -u):$(id -g)"`.
- **Rootless docker**: your host user maps to container *root*, so the default uid 1000 maps to a subordinate uid that can't read 0700 mounts like `~/.aws` — add `--user root` (which is still just your host user).

## Quick start

```sh
docker run --rm ghcr.io/dmallubhotla/kestrel:latest kest --version
```

For real work, run it from your project repo with your config mounted in:

```sh
docker run --rm -it \
  -v "$PWD:/work" \
  -v "$HOME/.config/kest:/home/kest/.config/kest" \
  -v "$HOME/.aws:/home/kest/.aws" \
  -v "$HOME/.kube:/home/kest/.kube" \
  ghcr.io/dmallubhotla/kestrel kest doctor
```

kest reads everything from a handful of well-known paths — each mount above maps one from your machine:

| Host path | Container path | Why |
| --- | --- | --- |
| your project repo | `/work` | `.kestconfig`, helm values, terraform roots, and the git history the guards and tag resolution read |
| `~/.config/kest` | `/home/kest/.config/kest` | global config (`config.yaml`: account→profile map, cluster contexts) |
| `~/.aws` | `/home/kest/.aws` | profiles for SSO and the SSO token cache (add `:ro` if you log in on the host; SSO login from inside the container needs it writable to refresh the token cache) |
| `~/.kube` | `/home/kest/.kube` | kube contexts (kest expects the contexts named in your global config to exist here) |
| `~/.local/state/kest` | `/home/kest/.local/state/kest` | optional — persists `kest profile set` between runs (each `docker run` is otherwise a fresh container) |

Put together, an alias does the job:

```sh
alias dkest='docker run --rm -it \
  -v "$PWD:/work" \
  -v "$HOME/.config/kest:/home/kest/.config/kest" \
  -v "$HOME/.aws:/home/kest/.aws" \
  -v "$HOME/.kube:/home/kest/.kube" \
  -v "$HOME/.local/state/kest:/home/kest/.local/state/kest" \
  ghcr.io/dmallubhotla/kestrel kest'

dkest doctor
dkest swoop
```

(Add `--user root` for rootless docker, or `--user "$(id -u):$(id -g)"` for rootful docker when your uid isn't 1000 — see above.)

`-it` matters for anything interactive (the swoop TUI, prompts). The image pre-trusts `/work` in git's `safe.directory`, so mounted repos with host-owned files work out of the box; if you mount your repo elsewhere, add `-e GIT_CONFIG_COUNT=1 -e GIT_CONFIG_KEY_0=safe.directory -e GIT_CONFIG_VALUE_0=<path>`.

### AWS SSO in a container

`aws sso login` can't open a browser inside the container. Either log in on the host and share the token cache (the `~/.aws` mount above covers it — kest's `auto_sso_login` then just works against the cached session), or run `dkest exec -- aws sso login --no-browser ...` and follow the device-code URL by hand.

### Terraform provider cache (optional)

Each container starts empty, so `terraform init` re-downloads providers every run. Persist them with a named volume:

```sh
-v kest-tf-plugins:/home/kest/.terraform.d/plugin-cache \
-e TF_PLUGIN_CACHE_DIR=/home/kest/.terraform.d/plugin-cache
```

## kestci in CI

`kestci` needs no mounts beyond the repo: AWS credentials come from the environment (OIDC or env vars), and the kubeconfig comes from the environment (a `KUBECONFIG` secret or a target's explicit `kubeconfig:`). It refuses to run unless `CI=true` (GitHub Actions sets this automatically).

Run the job inside the image and the whole toolchain is already there:

```yaml
jobs:
  deploy:
    runs-on: ubuntu-latest
    container:
      image: ghcr.io/dmallubhotla/kestrel:0
      # The runner mounts the workspace owned by its own user; non-root
      # container users can't write it (actions/checkout#956).
      options: --user root
    permissions:
      id-token: write
      contents: read
    steps:
      - uses: actions/checkout@v6
      - uses: aws-actions/configure-aws-credentials@v5
        with:
          role-to-assume: arn:aws:iam::111122223333:role/deploy
          aws-region: us-east-1
      - run: kestci deploy --all --target dev
```

Keep related steps (deploy + verification) in the same job so they share the same kubeconfig from the environment.

## Building locally

Linux only (`dockerTools`):

```sh
just docker        # nix build .#docker + docker load
just docker-exec   # bash inside the freshly built image
just docker-dive   # layer-by-layer size exploration
just dkest <args>  # kest from the freshly built image, with the dkest alias mounts
```

The image is defined in `flake.nix` (`dockerImageFor`); its version tag and OCI labels are stamped from the flake `version`, which `hanko seal` maintains. Release publishing happens in CI on `v*` tags via [`scripts/deploy-image.sh`](../scripts/deploy-image.sh).
