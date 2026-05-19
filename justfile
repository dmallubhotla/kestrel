# executes default, set to listing commands
default:
    just --list

# build the kest binary via nix
build:
    nix build

# run go tests via nix develop
test:
    nix flake check
    # go test ./...

# run nix flake check
check:
    nix flake check

# format nix files
fmt:
    nix fmt

# update flake inputs
update:
    nix flake update

# regenerate gomod2nix.toml and tidy go modules
chores:
    nix develop --command go mod tidy
    nix develop --command gomod2nix

# bump flake.nix to the hanko-computed semver, commit, tag, push
release:
    #!/usr/bin/env bash
    set -euo pipefail
    if [ -n "$(git status --porcelain)" ]; then
        echo "worktree dirty; commit or stash before releasing" >&2
        exit 1
    fi
    nix develop --command hanko stamp nix
    ver=$(nix develop --command hanko version)
    git add flake.nix
    git commit -m "Release ${ver}"
    nix develop --command hanko tag --push
