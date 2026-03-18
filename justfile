# executes default, set to listing commands
default:
    just --list

# build the kest binary via nix
build:
    nix build

# run go tests via nix develop
test:
    nix develop --command go test ./...

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
