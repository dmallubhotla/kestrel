# executes default, set to listing commands
default:
    just --list

# build the kest binary via nix
build:
    nix build

# build the docker image (linux only) as a docker-archive at ./result-docker
docker-build:
    nix build .#docker -o result-docker

# load the built docker image into docker
docker-load:
    docker load < result-docker

# build and load in one step
docker: docker-build docker-load

# shell into the most recently built image
docker-exec:
    docker run -it --rm "$(docker load -q < result-docker | sed -n 's/^Loaded image: //p')" bash

# explore the image layers with dive
docker-dive:
    nix run nixpkgs#dive -- "$(docker load -q < result-docker | sed -n 's/^Loaded image: //p')"

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

# release: stamp flake.nix, commit, tag, push — all via `hanko seal`.
# Reads .hanko.yaml for stamp-targets + seal config.
# Preview with `just release-plan` before running for real.
release:
    nix develop --command hanko seal

# release-plan: print what `just release` would do without mutating anything.
release-plan:
    nix develop --command hanko seal --dry-run
