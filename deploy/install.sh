#!/bin/sh
# Install and manage a Vestibule deployment from a GitHub release.
set -eu

repo=Zakkaus/vestibule
name=vestibule
action=deploy
action_set=no
deployment=container
deployment_set=no
version=
purge_data=ask
root=${VESTIBULE_ROOT:-}
fetch_override=${VESTIBULE_FETCH:-}
systemctl_override=${VESTIBULE_SYSTEMCTL:-}
container_runtime_override=${VESTIBULE_DOCKER:-}
claim_base_url=${VESTIBULE_CLAIM_BASE_URL:-http://127.0.0.1:8080}

usage() {
	cat <<'EOF'
Usage: install.sh [--container|--native] [--upgrade] [VERSION]
       install.sh [--container|--native] --rollback
       install.sh [--container|--native] --uninstall [--purge-data|--keep-data]
       install.sh [--container|--native] --status

Container deployment is the default. Native deployment remains available through
--native. Container deployment requires a preconfigured bot-api.env with the
upstream Bot API's TELEGRAM_API_ID and TELEGRAM_API_HASH; this installer never
asks for or invents credentials. Uninstall preserves configuration and state
unless --purge-data is explicitly selected.
EOF
}

fail() { printf 'install.sh: %s\n' "$*" >&2; exit 1; }
warn() { printf 'install.sh: WARNING: %s\n' "$*" >&2; }

select_action() {
	requested=$1
	if [ "$action_set" = yes ] && [ "$action" != "$requested" ]; then
		fail "choose only one lifecycle action"
	fi
	action=$requested
	action_set=yes
}

select_deployment() {
	requested=$1
	if [ "$deployment_set" = yes ] && [ "$deployment" != "$requested" ]; then
		fail "choose only one deployment"
	fi
	deployment=$requested
	deployment_set=yes
}

while [ "$#" -gt 0 ]; do
	case $1 in
		-h | --help) usage; exit 0 ;;
		--upgrade) select_action upgrade ;;
		--rollback) select_action rollback ;;
		--uninstall) select_action uninstall ;;
		--status) select_action status ;;
		--container) select_deployment container ;;
		--native) select_deployment native ;;
		--purge-data) purge_data=yes ;;
		--keep-data) purge_data=no ;;
		-*) fail "unknown option $1" ;;
		*)
			[ -z "$version" ] || fail "only one release version may be specified"
			version=$1
			;;
	esac
	shift
done

case $root in
	/) root= ;;
	'' | /*) ;;
	*) fail "VESTIBULE_ROOT must be an absolute path" ;;
esac
case $root in
	*'|'*) fail "VESTIBULE_ROOT may not contain a pipe character" ;;
esac
case $claim_base_url in
	http://* | https://*) claim_base_url=${claim_base_url%/} ;;
	*) fail "VESTIBULE_CLAIM_BASE_URL must be an absolute HTTP or HTTPS URL" ;;
esac

prefix=${root}/usr/local/bin
libexec=${root}/usr/local/libexec
confdir=${root}/etc/${name}
state_dir=${root}/var/lib/${name}
bot_api_state_dir=${root}/var/lib/${name}-bot-api
binary=${prefix}/${name}
previous_binary=${prefix}/${name}.previous
unit=${root}/etc/systemd/system/${name}.service
previous_unit=${unit}.previous
replacement_service=${root}/etc/systemd/system/${name}-replace.service
replacement_path=${root}/etc/systemd/system/${name}-replace.path
replacement_path_name=${name}-replace.path
replacement_runner=${libexec}/${name}-replace
replacement_unit_state=${state_dir}/replacement-unit.env
managed_installer=${libexec}/${name}-install
managed_common=${libexec}/${name}-install-common
managed_native=${libexec}/${name}-install-native
managed_container=${libexec}/${name}-install-container
bot_env=${confdir}/bot.env
bot_api_env=${confdir}/bot-api.env
config_file=${confdir}/config.json
compose_file=${confdir}/compose.yaml
container_env=${confdir}/container.env
previous_container_env=${container_env}.previous
deployment_file=${confdir}/deployment.env
setup_env=${confdir}/setup.env
current_manifest=${confdir}/schema-manifest
previous_manifest=${confdir}/schema-manifest.previous
current_version_file=${confdir}/release-version
previous_version_file=${confdir}/release-version.previous
claim_state=${state_dir}/claim.json
result_file=${confdir}/install-result.env
work=
txn_active=no
txn_count=0
journal=
dir_journal=
manager=foreground
systemctl_bin=
container_runtime=
fetcher=
service_changed=no
replacement_unit_changed=no
container_changed=no
had_binary_before=no
had_unit_before=no
unit_enabled_before=unknown
unit_active_before=unknown
had_replacement_path_before=no
had_container_before=no
claim_url=
setup_hash=
new_claim_expected=no

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd) || fail "cannot resolve installer directory"
if [ -r "${script_dir}/install-common.sh" ]; then
	common_library=${script_dir}/install-common.sh
	native_library=${script_dir}/install-native.sh
	container_library=${script_dir}/install-container.sh
else
	common_library=${script_dir}/${name}-install-common
	native_library=${script_dir}/${name}-install-native
	container_library=${script_dir}/${name}-install-container
fi
for library in "$common_library" "$native_library" "$container_library"; do
	[ -r "$library" ] || fail "installer support file is missing: $library"
done
. "$common_library"
. "$native_library"
. "$container_library"

read_deployment_record() {
	[ -f "$deployment_file" ] || return 1
	case $(cat "$deployment_file") in
		deployment=native) installed_deployment=native ;;
		deployment=container) installed_deployment=container ;;
		*) fail "$deployment_file is malformed" ;;
	esac
}

select_current_deployment() {
	if [ "$deployment_set" = yes ]; then
		if [ -f "$deployment_file" ]; then
			read_deployment_record
			[ "$deployment" = "$installed_deployment" ] || fail "existing deployment is $installed_deployment, not $deployment"
		elif [ -e "$binary" ] && [ "$deployment" != native ]; then
			fail "existing native files have no deployment record; do not change deployment in place"
		fi
		return
	fi
	if [ -f "$deployment_file" ]; then
		read_deployment_record
		deployment=$installed_deployment
	elif [ -e "$binary" ]; then
		deployment=native
	fi
}

select_current_deployment
case $action in
	deploy | upgrade)
		[ "$purge_data" = ask ] || fail "--purge-data and --keep-data apply only to --uninstall"
		case $deployment in
			native) native_install_or_upgrade ;;
			container) container_install_or_upgrade ;;
		esac
		;;
	rollback)
		[ -z "$version" ] || fail "--rollback does not accept a release version"
		[ "$purge_data" = ask ] || fail "data options apply only to --uninstall"
		case $deployment in
			native) native_rollback_release ;;
			container) container_rollback_release ;;
		esac
		;;
	status)
		[ -z "$version" ] || fail "--status does not accept a release version"
		[ "$purge_data" = ask ] || fail "data options apply only to --uninstall"
		case $deployment in
			native) native_show_status ;;
			container) container_show_status ;;
		esac
		;;
	uninstall)
		[ -z "$version" ] || fail "--uninstall does not accept a release version"
		case $deployment in
			native) native_uninstall_release ;;
			container) container_uninstall_release ;;
		esac
		;;
esac
