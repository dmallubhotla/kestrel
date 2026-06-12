#!/usr/bin/env bash
set -Eeuo pipefail

# Publish nix-built docker archives to a registry, assembling a multi-arch
# manifest for every ref hanko emits. Run from a checkout of the release
# tag with full history (hanko computes the version from git).
#
# Tag policy lives here at the call site (hanko emitters take flags, not
# config):
#   - semver fan-out (X.Y.Z, X.Y, X) from the checked-out release tag
#   - explicit `latest`: kestrel only seals from master, so a v* tag
#     implies mainline — but a CI tag checkout is detached, and hanko
#     (D-001) emits the tag's version without knowing the branch, so
#     `:latest` must be requested explicitly.
#   - no branch-sha ref: `detached-<sha>` is noise on a release.
#
# Needs hanko (e.g. `nix develop --command bash scripts/deploy-image.sh`),
# plus skopeo, jq, and docker (for buildx imagetools) — all preinstalled
# on GitHub ubuntu runners.
#
# Usage: deploy-image.sh <docker-archive>...
#   e.g. deploy-image.sh kest-docker-linux-amd64.tar.gz kest-docker-linux-arm64.tar.gz
#
# Configuration (environment variables):
#   IMAGE               image repository (default ghcr.io/dmallubhotla/kestrel)
#   REGISTRY_USER       login user (login skipped if unset)
#   REGISTRY_PASSWORD   login password/token

IMAGE=${IMAGE:-"ghcr.io/dmallubhotla/kestrel"}
REGISTRY="${IMAGE%%/*}"

# Collapsible log sections: GitHub Actions output groups on CI, plain
# headers elsewhere. Close any open group before starting the next.
group() {
  if [[ -n ${GITHUB_ACTIONS:-} ]]; then
    echo "::group::$*"
  else
    echo "=== $* ==="
  fi
}
endgroup() {
  if [[ -n ${GITHUB_ACTIONS:-} ]]; then
    echo "::endgroup::"
  fi
}

if [[ $# -lt 1 ]]; then
  echo "Error: no docker archives given. Build with 'nix build .#docker' first." >&2
  exit 1
fi

for archive in "$@"; do
  if [[ ! -f ${archive} ]]; then
    echo "Error: docker archive '${archive}' not found." >&2
    exit 1
  fi
done

mapfile -t REFS < <(hanko version docker tags "${IMAGE}" --branch-sha-tag=false --extra latest)
echo "Deploying refs: ${REFS[*]}"

if [[ -n ${REGISTRY_USER:-} && -n ${REGISTRY_PASSWORD:-} ]]; then
  group "Logging in to ${REGISTRY}"
  echo "${REGISTRY_PASSWORD}" | skopeo login --username "${REGISTRY_USER}" --password-stdin "${REGISTRY}"
  echo "${REGISTRY_PASSWORD}" | docker login --username "${REGISTRY_USER}" --password-stdin "${REGISTRY}"
  endgroup
fi

# Push each archive under an arch-suffixed anchor tag, then assemble a
# multi-arch manifest per ref pointing at the anchors.
ANCHOR_REFS=()
for archive in "$@"; do
  ARCH=$(skopeo inspect "docker-archive:${archive}" | jq -r '.Architecture')
  ANCHOR="${REFS[0]}-${ARCH}"
  group "Pushing ${archive} (${ARCH}) -> ${ANCHOR}"
  skopeo copy --insecure-policy "docker-archive:${archive}" "docker://${ANCHOR}"
  endgroup
  ANCHOR_REFS+=("${ANCHOR}")
done

for ref in "${REFS[@]}"; do
  group "Creating manifest ${ref}"
  docker buildx imagetools create --tag "${ref}" "${ANCHOR_REFS[@]}"
  endgroup
done

echo "Deployment complete: ${REFS[*]}"
