#!/usr/bin/env bash

set -euo pipefail

function usage() {
	cat <<-EOF
		Usage: $(basename "${BASH_SOURCE[0]}") [-h] [-l logfile] [-v] [-t tag] target_env release_name [ additional_args ]

		Deploys helm to the target environment with provided release name.

		Makes assumptions about the existence of a values file with the same name as target_env, among other things.

		Additional arguments passed after a release name will be provided as-is to the helm upgrade command.
		However, if the first provided additional argument is either "ls" or "uninstall", then the script will not release the chart.
		Instead, for "ls" deployment information will be listed.
		If "uninstall" is provided, the helm deploy will be removed.

		Available options:
		-h, --help                Print help!
		-l, --logfile <file>      Choose a log file! (only stderr if not set)
		-v, --verbose             Be a bit more verbose!
		-t, --tag <tag>           Override the app deployment image tag with a specified value
		--override-namespace <ns> Override the namespace.
		--override-chart <chart>  Overrides the chart
	EOF
	exit
}

# send a message to stdout and tee it to a file if log is set
function log() {
	if [[ -n "${_logfile:-}" ]]; then
		echo "$1" | tee -a "${_logfile}"
	else
		echo "$1"
	fi
}

# log a message and exit bad
function fail() {
	log "${1}"
	exit 1
}

# We can't use getopts to parse params because that's not cross platform
# getopt is cross-platform, but doesn't support long (that is, legible) options
parse_args() {
	# default values of variables set from params
	_logfile=''
	_verbose=0
	_namespace='app'
	_tagOverride=''
	_chart='oci://ghcr.io/example-org/example-charts/example-ms'

	while :; do
		case "${1-}" in
			-h | --help) usage ;;
			-v | --verbose)
				set -x
				_verbose=1
				;;
			-l | --logfile)
				_logfile="${2-}"
				shift
				;;
			-t | --tag)
				_tagOverride="${2-}"
				shift
				;;
			--override-namespace)
				_namespace="${2-}"
				shift
				;;
			--override-chart)
				_chart="${2-}"
				shift
				;;
			-?*) fail "Unknown option: $1" ;;
			*) break ;;
		esac
		shift
	done


	args=("$@")
	# Here 2 because we have 2 required positional args.
	# There are better ways of parsing arguments, but not worth it here.
	[[ ${#args[@]} -lt 2 ]] && {
		fail "Missing required argument! Must call like `common-helm-deploy.sh target_env release_name`"
	}

	target_env="$1"
	case $target_env in
		prod)   _kube_context='arn:aws:eks:us-east-1:593671994769:cluster/eks-prod';;
		stage)  _kube_context='arn:aws:eks:us-east-1:593671994769:cluster/eks-stage' ;;
		dev)    _kube_context='arn:aws:eks:us-east-1:585912155334:cluster/eks-dev' ;;
		*)      fail "Invalid argument for target environment!  Must target one of <dev|stage|prod>";;
	esac
	shift

	release_name="$1"
	shift

	extra_args=("$@")
	return 0
}

parse_args "$@"

# print out any parsed arguments
if [[ "${_verbose}" -eq 1 ]]; then
	log "target env: ${target_env}"
	log "release name env: ${target_env}"
	log "logfile: ${_logfile:-}"
	log "namespace: ${_namespace}"
	log "args: [${args:-}]"
	log "extra args: [${extra_args:-}]"
fi

# make sure our dependencies are available (some environments are not yet supported)
[[ -a ./${target_env}.yaml ]] || fail "Target environment ${target_env} not yet supported, no values file found"
[[ -a $(which helm 2>/dev/null) ]] || fail "you must have helm installed to use this script"

# Some magic if the first additional argument is uninstall
if [ "${extra_args[0]:-}" == "uninstall" ]; then
	log "removing ${release_name}"
	helm uninstall \
		--namespace ${_namespace} \
		--wait \
		--timeout 5m0s \
		--kube-context ${_kube_context} \
		${release_name}
	exit
fi

# display the deployed application details if the first additional argument is ls
if [ "${extra_args[0]:-}" == "ls" ]; then
	log "retrieving deployment state..."
	helm ls --kube-context ${_kube_context} -n ${_namespace} | grep ${release_name}
	exit
fi

#determine the current db connection, whether or not we need to build a cluster
# and whether or not we can use auto DB credentials
_db_mode=${target_env}
_db_conn="auto"

if [ "${target_env}" == "prod" ]; then
	_tag=$(git describe --tags `git rev-list --tags --max-count=1`)
elif [ -n "${_tagOverride}" ]; then
	_tag=${_tagOverride}
else
	# try to figure out a tag from git info
	_tag=$(git rev-parse --abbrev-ref HEAD)-$(git rev-parse --short HEAD)
fi
log "Deploying container version: ${_tag}"

# we may want to allow multiple values files
VALUES_FLAGS=()
if [[ -f "shared.yaml" ]]; then
	VALUES_FLAGS+=(--values shared.yaml)
	log "Found a shared.yaml file, including in helm upgrade"
fi
# we want the target env values file loaded after the shared
VALUES_FLAGS+=(--values ./"${target_env}.yaml")

#run the actual installation of the chart
# note that in helm 4 --atomic should be replaced with --wait and --rollback-on-failure
helm upgrade \
	--namespace ${_namespace} \
	--atomic \
	--cleanup-on-fail \
	--install \
	--history-max 0 \
	--timeout 5m0s \
	--kube-context "${_kube_context}" \
	"${VALUES_FLAGS[@]}" \
	--set "env.dbConn=${_db_conn}" \
	--set "image.tag=${_tag}" \
	"${release_name}" "${_chart}" "${extra_args[@]}" || fail "helm failed to deploy to ${target_env}"

# display the deployed application details
helm ls --kube-context ${_kube_context} -n ${_namespace}
