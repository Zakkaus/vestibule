# Shared transaction, release, and host-unit helpers for install.sh.

service_control() { [ "$manager" = systemd ] || return 0; "$systemctl_bin" "$@"; }

restore_transaction() {
	set +e
	if [ -n "$journal" ] && [ -f "$journal" ]; then
		while IFS='|' read -r disposition destination backup; do
			[ -n "$disposition" ] || continue
			case $disposition in
				restore) cp -p -- "$backup" "$destination" ;;
				remove) rm -f -- "$destination" ;;
			esac
		done < "$journal"
	fi
	if [ "$container_changed" = yes ]; then
		restore_container_runtime
	fi
	if [ "$replacement_unit_changed" = yes ] && [ "$manager" = systemd ]; then
		service_control daemon-reload >/dev/null 2>&1 || :
		if [ "$had_replacement_path_before" = no ]; then
			service_control disable --now "$replacement_path_name" >/dev/null 2>&1 || :
		fi
	fi
	if [ "$service_changed" = yes ] && [ "$manager" = systemd ]; then
		service_control daemon-reload >/dev/null 2>&1 || :
		if [ "$had_unit_before" = yes ] && [ "$had_binary_before" = yes ]; then
			case $unit_enabled_before in
				enabled|enabled-runtime|alias|static|indirect|generated)
					service_control enable "$name" >/dev/null 2>&1 || : ;;
				disabled|masked)
					service_control disable "$name" >/dev/null 2>&1 || : ;;
			esac
			if [ "$unit_active_before" = inactive ] || [ "$unit_active_before" = failed ]; then
				service_control stop "$name" >/dev/null 2>&1 || :
			else
				service_control restart "$name" >/dev/null 2>&1 || :
			fi
		else
			service_control disable --now "$name" >/dev/null 2>&1 || :
		fi
	fi
	if [ "$new_claim_expected" = yes ] && [ -f "$claim_state" ]; then
		actual_claim=$(cat "$claim_state")
		[ "$actual_claim" != "{\"setup_token_hash\":\"${setup_hash}\"}" ] || rm -f -- "$claim_state"
	fi
	if [ -n "$dir_journal" ] && [ -f "$dir_journal" ]; then
		while IFS= read -r directory; do
			[ -n "$directory" ] && rmdir -- "$directory" >/dev/null 2>&1 || :
		done < "$dir_journal"
	fi
	set -e
}

on_exit() {
	status=$?
	trap - EXIT HUP INT TERM
	if [ "$status" -ne 0 ] && [ "$txn_active" = yes ]; then
		warn "operation failed; restoring existing files and removing only files created by this run"
		restore_transaction
	fi
	[ -z "$work" ] || rm -rf -- "$work"
	exit "$status"
}

trap on_exit EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

make_workdir() {
	if [ -n "$work" ]; then return 0; fi
	work=$(mktemp -d "${TMPDIR:-/tmp}/vestibule-install.XXXXXX") || fail "cannot create temporary directory"
	journal=${work}/files.journal
	dir_journal=${work}/dirs.journal
	: > "$journal"
	: > "$dir_journal"
}

begin_transaction() {
	make_workdir
	txn_active=yes
	if [ -e "$binary" ]; then
		had_binary_before=yes
	fi
	if [ -e "$unit" ]; then
		had_unit_before=yes
		# Whether the unit was enabled and running is state a rollback has to put back.
		# The native installer disables the service before it replaces anything, so a
		# rollback that only restarts leaves it running now and not after a reboot.
		unit_enabled_before=$(service_control is-enabled "$name" 2>/dev/null || echo unknown)
		unit_active_before=$(service_control is-active "$name" 2>/dev/null || echo unknown)
	fi
	if [ -e "$replacement_path" ]; then
		had_replacement_path_before=yes
	fi
	if [ -e "$container_env" ]; then
		had_container_before=yes
	fi
}

commit_transaction() { txn_active=no; }

ensure_directory() {
	directory=$1
	if [ -d "$directory" ]; then
		return
	fi
	[ ! -e "$directory" ] || fail "$directory exists and is not a directory"
	mkdir -p -- "$directory"
	printf '%s\n' "$directory" >> "$dir_journal"
}

txn_replace() {
	source_file=$1
	mode=$2
	destination=$3
	ensure_directory "${destination%/*}"
	txn_count=$((txn_count + 1))
	backup=${work}/backup.${txn_count}
	if [ -e "$destination" ] || [ -L "$destination" ]; then
		cp -p -- "$destination" "$backup"
		printf 'restore|%s|%s\n' "$destination" "$backup" >> "$journal"
	else
		printf 'remove|%s|\n' "$destination" >> "$journal"
	fi
	staged=${destination}.vestibule-new.$$
	rm -f -- "$staged"
	cp -- "$source_file" "$staged"
	chmod "$mode" "$staged"
	mv -f -- "$staged" "$destination"
}

txn_remove() {
	destination=$1
	if [ ! -e "$destination" ] && [ ! -L "$destination" ]; then
		return
	fi
	txn_count=$((txn_count + 1))
	backup=${work}/backup.${txn_count}
	cp -p -- "$destination" "$backup"
	printf 'restore|%s|%s\n' "$destination" "$backup" >> "$journal"
	rm -f -- "$destination"
}

require_mutation_access() {
	if [ -z "$root" ] && [ "$(id -u)" -ne 0 ]; then
		fail "install, upgrade, rollback, and uninstall require root; rerun this command through sudo"
	fi
}

detect_fetcher() {
	if [ -n "$fetch_override" ]; then
		command -v "$fetch_override" >/dev/null 2>&1 || fail "VESTIBULE_FETCH is not executable: $fetch_override"
		fetcher=override
	elif command -v curl >/dev/null 2>&1; then
		fetcher=curl
	elif command -v wget >/dev/null 2>&1; then
		fetcher=wget
	else
		fail "neither curl nor wget is available"
	fi
}

fetch_url() {
	url=$1
	destination=$2
	case $fetcher in
		override) "$fetch_override" "$url" "$destination" ;;
		curl) curl --fail --silent --show-error --location --output "$destination" "$url" ;;
		wget) wget --quiet --output-document="$destination" "$url" ;;
	esac
}

detect_service_manager() {
	if [ -n "$systemctl_override" ]; then
		command -v "$systemctl_override" >/dev/null 2>&1 || fail "VESTIBULE_SYSTEMCTL is not executable: $systemctl_override"
		systemctl_bin=$systemctl_override
		manager=systemd
	elif [ -z "$root" ] && command -v systemctl >/dev/null 2>&1; then
		systemctl_bin=$(command -v systemctl)
		manager=systemd
	else
		manager=foreground
	fi
}

detect_architecture() {
	case $(uname -m) in
		x86_64) arch=amd64 ;;
		aarch64 | arm64) arch=arm64 ;;
		*) fail "no release binary for $(uname -m)" ;;
	esac
}

validate_version() {
	case $version in
		'' | *[!A-Za-z0-9._-]*) fail "invalid release version: $version" ;;
	esac
}

resolve_version() {
	[ -n "$version" ] && { validate_version; return; }
	fetch_url "https://api.github.com/repos/${repo}/releases/latest" "${work}/latest.json"
	version=$(sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' "${work}/latest.json" | sed -n '1p')
	[ -n "$version" ] || fail "could not resolve the latest release"
	validate_version
}

verify_asset() {
	asset=$1
	expected=$(awk -v name="$asset" '$2 == name { count++; digest=$1 } END { if (count != 1) exit 1; print digest }' "${work}/SHA256SUMS") \
		|| fail "SHA256SUMS must contain exactly one entry for $asset"
	[ "${#expected}" -eq 64 ] || fail "SHA256SUMS has an invalid digest for $asset"
	case $expected in *[!0-9a-fA-F]*) fail "SHA256SUMS has an invalid digest for $asset" ;; esac
	actual=$(sha256sum "${work}/${asset}")
	actual=${actual%% *}
	[ "$actual" = "$expected" ] || fail "checksum mismatch for $asset"
	printf 'verified SHA-256: %s\n' "$asset"
}

parse_manifest() {
	manifest_path=$1
	parsed_target=
	parsed_minimum=
	seen_target=no
	seen_minimum=no
	while IFS= read -r line || [ -n "$line" ]; do
		case $line in
			target_schema_version=*)
				[ "$seen_target" = no ] || fail "$manifest_path repeats target_schema_version"
				parsed_target=${line#*=}
				seen_target=yes
				;;
			minimum_rollback_schema_version=*)
				[ "$seen_minimum" = no ] || fail "$manifest_path repeats minimum_rollback_schema_version"
				parsed_minimum=${line#*=}
				seen_minimum=yes
				;;
			*) fail "$manifest_path contains an unknown or malformed field" ;;
		esac
	done < "$manifest_path"
	[ "$seen_target" = yes ] && [ "$seen_minimum" = yes ] || fail "$manifest_path is incomplete"
	case $parsed_target in '' | *[!0-9]*) fail "$manifest_path has an invalid target schema version" ;; esac
	case $parsed_minimum in '' | *[!0-9]*) fail "$manifest_path has an invalid rollback floor" ;; esac
	[ "$parsed_minimum" -le "$parsed_target" ] || fail "$manifest_path has a rollback floor above its target"
}

fetch_release_manifest() {
	base="https://github.com/${repo}/releases/download/${version}"
	fetch_url "${base}/SHA256SUMS" "${work}/SHA256SUMS"
	fetch_url "${base}/${name}-schema-manifest" "${work}/${name}-schema-manifest"
	warn "SHA256SUMS comes from the same GitHub release as the assets; it verifies integrity against that file, not publisher authenticity, because no detached signature is published"
	verify_asset "${name}-schema-manifest"
	parse_manifest "${work}/${name}-schema-manifest"
	target_schema=$parsed_target
	target_minimum=$parsed_minimum
}

fetch_release_support() {
	for asset in \
		"${name}.service" \
		"${name}-install" \
		"${name}-install-common" \
		"${name}-install-native" \
		"${name}-install-container" \
		"${name}-replace" \
		"${name}-replace.service" \
		"${name}-replace.path" \
		compose.yaml; do
		fetch_url "${base}/${asset}" "${work}/${asset}"
		verify_asset "$asset"
	done
}

fetch_release_payload() {
	binary_asset=${name}-linux-${arch}
	fetch_url "${base}/${binary_asset}" "${work}/${binary_asset}"
	verify_asset "$binary_asset"
}

install_support_files() {
	txn_replace "${work}/${name}-install" 755 "$managed_installer"
	txn_replace "${work}/${name}-install-common" 644 "$managed_common"
	txn_replace "${work}/${name}-install-native" 644 "$managed_native"
	txn_replace "${work}/${name}-install-container" 644 "$managed_container"
	txn_replace "${work}/${name}-replace" 755 "$replacement_runner"
	txn_replace "${work}/${name}-replace.service" 644 "$replacement_service"
	txn_replace "${work}/${name}-replace.path" 644 "$replacement_path"
	replacement_unit_changed=yes
	if [ "$manager" = systemd ]; then
		service_control daemon-reload
		service_control enable --now "$replacement_path_name"
	else
		warn "systemctl was not found; the replacement unit is installed but unavailable"
	fi
}

write_replacement_unit_state() {
	[ "$manager" = systemd ] || return 0
	[ -d "$state_dir" ] || fail "service started without creating $state_dir"
	printf 'available=yes\n' > "${work}/replacement-unit.env"
	txn_replace "${work}/replacement-unit.env" 600 "$replacement_unit_state"
	chown --reference="$state_dir" "$replacement_unit_state" 2>/dev/null || :
}

ensure_bot_env() {
	if [ ! -e "$bot_env" ]; then
		: > "${work}/bot.env"
		txn_replace "${work}/bot.env" 600 "$bot_env"
	fi
}

random_setup_token() {
	if command -v openssl >/dev/null 2>&1; then
		openssl rand -hex 32
	elif command -v od >/dev/null 2>&1; then
		od -An -N32 -tx1 /dev/urandom | tr -d ' \n'
	else
		fail "openssl or od is required to generate the one-use claim link"
	fi
}

result_value() {
	key=$1
	[ -f "$result_file" ] || return 0
	awk -F= -v key="$key" '$1 == key { sub(/^[^=]*=/, ""); print; exit }' "$result_file"
}

write_result() {
	result_operation=$1
	result_version=$2
	rollback_available=$3
	result_deployment=$4
	[ -n "$claim_url" ] || claim_url=$(result_value claim_url)
	cat > "${work}/install-result.env" <<EOF
operation=${result_operation}
version=${result_version}
deployment=${result_deployment}
claim_url=${claim_url}
rollback_available=${rollback_available}
checksum_integrity=sha256
checksum_authenticity=unverified_same_release
EOF
	txn_replace "${work}/install-result.env" 600 "$result_file"
}

prepare_first_claim() {
	claim_url=$(result_value claim_url)
	if [ -e "$claim_state" ]; then
		rm -f -- "$setup_env"
		return
	fi
	if [ -n "$claim_url" ]; then
		setup_token=${claim_url##*/}
		[ "${#setup_token}" -eq 64 ] || fail "$result_file contains an invalid claim URL"
		case $setup_token in *[!0-9a-f]*) fail "$result_file contains an invalid claim URL" ;; esac
	else
		setup_token=$(random_setup_token)
		claim_url=${claim_base_url}/setup/${setup_token}
	fi
	setup_hash=$(printf '%s' "$setup_token" | sha256sum)
	setup_hash=${setup_hash%% *}
	if [ "$manager" = systemd ]; then
		printf 'SETUP_TOKEN=%s\n' "$setup_token" > "${work}/setup.env"
		txn_replace "${work}/setup.env" 600 "$setup_env"
		new_claim_expected=yes
	else
		ensure_directory "$state_dir"
		printf '{"setup_token_hash":"%s"}\n' "$setup_hash" > "${work}/claim.json"
		txn_replace "${work}/claim.json" 600 "$claim_state"
	fi
}
