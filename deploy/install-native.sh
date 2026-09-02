# Native lifecycle actions for install.sh.

stage_previous_release() {
	cp -p -- "$binary" "${work}/retained-binary"
	cp -p -- "$current_manifest" "${work}/retained-manifest"
	cp -p -- "$current_version_file" "${work}/retained-version"
	txn_replace "${work}/retained-binary" 755 "$previous_binary"
	txn_replace "${work}/retained-manifest" 644 "$previous_manifest"
	txn_replace "${work}/retained-version" 644 "$previous_version_file"
	if [ -f "$unit" ]; then
		cp -p -- "$unit" "${work}/retained-unit"
		txn_replace "${work}/retained-unit" 644 "$previous_unit"
	fi
}

native_install_or_upgrade() {
	require_mutation_access
	make_workdir
	detect_fetcher
	detect_service_manager
	detect_architecture
	operation=install
	if [ -e "$binary" ]; then
		operation=upgrade
	fi
	if [ "$action" = upgrade ] && [ "$operation" != upgrade ]; then
		fail "--upgrade requires an existing $binary"
	fi
	resolve_version
	printf '%s %s %s (%s)\n' "$operation" "$name" "$version" "$arch"
	fetch_release_manifest
	if [ "$operation" = upgrade ]; then
		assess_upgrade_rollback
	fi
	fetch_release_support
	fetch_release_payload
	begin_transaction
	ensure_directory "$prefix"
	ensure_directory "$confdir"
	ensure_bot_env
	prepare_first_claim
	if [ "$operation" = upgrade ]; then
		[ -f "$current_version_file" ] || fail "existing installation has no stored release version"
		stage_previous_release
	fi
	txn_replace "${work}/${name}-linux-${arch}" 755 "$binary"
	txn_replace "${work}/${name}-schema-manifest" 644 "$current_manifest"
	printf '%s\n' "$version" > "${work}/release-version"
	txn_replace "${work}/release-version" 644 "$current_version_file"
	install_support_files
	if [ "$manager" = systemd ]; then
		txn_replace "${work}/${name}.service" 644 "$unit"
		service_changed=yes
		service_control daemon-reload
		service_control enable "$name"
		service_control restart "$name"
		if [ "$new_claim_expected" = yes ]; then
			[ -f "$claim_state" ] || fail "service started without persisting the one-use claim"
			rm -f -- "$setup_env"
		fi
		write_replacement_unit_state
	else
		warn "systemctl was not found; files are installed, but no service was started or enabled"
		printf 'start manually: STATE_DIRECTORY=%s %s --config %s/config.json\n' "$state_dir" "$binary" "$confdir"
	fi
	if [ "$operation" = upgrade ]; then
		rollback_available=yes
	else
		rollback_available=no
	fi
	write_result "$operation" "$version" "$rollback_available" "native-${manager}"
	commit_transaction
	printf '%s %s complete\n' "$name" "$operation"
	claim_url=$(result_value claim_url)
	[ -z "$claim_url" ] || printf 'claim URL: %s\n' "$claim_url"
	printf 'result file: %s\n' "$result_file"
}

assess_upgrade_rollback() {
	[ -f "$current_manifest" ] || fail "existing installation has no stored schema manifest; rollback safety is unknown, so the binary will not be downloaded"
	parse_manifest "$current_manifest"
	current_schema=$parsed_target
	if [ "$target_schema" -lt "$current_schema" ]; then
		fail "target schema v${target_schema} is older than installed schema v${current_schema}; use --rollback with the retained release"
	fi
	if [ "$target_schema" -eq "$current_schema" ]; then
		printf 'rollback preflight: safe; target and retained release both use schema v%s\n' "$current_schema"
		return
	fi
	if [ "$current_schema" -lt "$target_minimum" ]; then
		fail "rollback preflight: unsafe; target schema v${target_schema} requires schema v${target_minimum} or newer, but the retained release uses v${current_schema}; the binary was not downloaded"
	fi
	printf 'rollback preflight: safe from target schema v%s to retained schema v%s (minimum v%s)\n' \
		"$target_schema" "$current_schema" "$target_minimum"
}

check_manual_rollback() {
	parse_manifest "$current_manifest"
	rollback_floor=$parsed_minimum
	current_schema=$parsed_target
	parse_manifest "$previous_manifest"
	rollback_schema=$parsed_target
	if [ "$rollback_schema" -lt "$rollback_floor" ]; then
		fail "retained release schema v${rollback_schema} is below current release rollback floor v${rollback_floor}"
	fi
	printf 'rollback preflight: safe from schema v%s to retained schema v%s\n' "$current_schema" "$rollback_schema"
}

swap_release_file() {
	current=$1
	previous=$2
	mode=$3
	label=$4
	cp -p -- "$current" "${work}/${label}.current"
	cp -p -- "$previous" "${work}/${label}.previous"
	txn_replace "${work}/${label}.previous" "$mode" "$current"
	txn_replace "${work}/${label}.current" "$mode" "$previous"
}

native_rollback_release() {
	require_mutation_access
	for required in "$binary" "$previous_binary" "$current_manifest" "$previous_manifest" \
		"$current_version_file" "$previous_version_file"; do
		[ -f "$required" ] || fail "cannot roll back; retained file is missing: $required"
	done
	make_workdir
	detect_service_manager
	check_manual_rollback
	begin_transaction
	swap_release_file "$binary" "$previous_binary" 755 binary
	swap_release_file "$current_manifest" "$previous_manifest" 644 manifest
	swap_release_file "$current_version_file" "$previous_version_file" 644 version
	if [ -f "$unit" ] && [ -f "$previous_unit" ]; then
		swap_release_file "$unit" "$previous_unit" 644 unit
	fi
	if [ "$manager" = systemd ]; then
		service_changed=yes
		service_control daemon-reload
		service_control restart "$name"
	else
		warn "systemctl was not found; release files were swapped, but no service was restarted"
	fi
	rolled_back_version=$(sed -n '1p' "$current_version_file")
	write_result rollback "$rolled_back_version" yes "native-${manager}"
	commit_transaction
	printf '%s rolled back to %s\n' "$name" "$rolled_back_version"
}

native_show_status() {
	detect_service_manager
	if [ ! -f "$binary" ]; then
		printf 'installed=no\n'
		return 1
	fi
	installed_version=unknown
	[ ! -f "$current_version_file" ] || installed_version=$(sed -n '1p' "$current_version_file")
	if [ -f "$previous_binary" ] && [ -f "$previous_manifest" ]; then
		rollback_available=yes
	else
		rollback_available=no
	fi
	printf 'installed=yes\nversion=%s\ndeployment=native-%s\nrollback_available=%s\n' \
		"$installed_version" "$manager" "$rollback_available"
	if [ -f "$replacement_unit_state" ] && grep -Fqx 'available=yes' "$replacement_unit_state"; then
		printf 'replacement_unit=available\n'
	else
		printf 'replacement_unit=unavailable\n'
	fi
	if [ "$manager" = systemd ]; then
		service_enabled=$(service_control is-enabled "$name" 2>/dev/null || :)
		service_active=$(service_control is-active "$name" 2>/dev/null || :)
		[ -n "$service_enabled" ] || service_enabled=unknown
		[ -n "$service_active" ] || service_active=unknown
		printf 'service_enabled=%s\nservice_active=%s\n' "$service_enabled" "$service_active"
	fi
	last_operation=$(result_value operation)
	claim_url=$(result_value claim_url)
	[ -z "$last_operation" ] || printf 'last_operation=%s\n' "$last_operation"
	[ -z "$claim_url" ] || printf 'claim_url=%s\n' "$claim_url"
	printf 'result_file=%s\n' "$result_file"
}

choose_purge_data() {
	if [ "$purge_data" != ask ]; then
		return
	fi
	purge_data=no
	if [ -t 0 ]; then
		printf 'Remove %s and %s, including credentials and database state? [y/N] ' "$confdir" "$state_dir"
		read -r answer || answer=
		case $answer in y | Y | yes | YES) purge_data=yes ;; esac
	fi
}

native_uninstall_release() {
	require_mutation_access
	make_workdir
	detect_service_manager
	choose_purge_data
	if [ "$manager" = systemd ] && [ -e "$unit" ]; then
		service_control disable --now "$name"
		service_changed=yes
	fi
	if [ "$manager" = systemd ] && [ -e "$replacement_path" ]; then
		service_control disable --now "$replacement_path_name"
		replacement_unit_changed=yes
	fi
	begin_transaction
	for installed_file in "$binary" "$previous_binary" "$unit" "$previous_unit" \
		"$current_manifest" "$previous_manifest" "$current_version_file" "$previous_version_file" \
		"$managed_installer" "$managed_common" "$managed_native" "$managed_container" \
		"$replacement_runner" "$replacement_service" "$replacement_path" "$replacement_unit_state"; do
		txn_remove "$installed_file"
	done
	if [ -d "$state_dir" ]; then
		uninstalled_version=$(result_value version)
		[ -n "$uninstalled_version" ] || uninstalled_version=unknown
		write_result uninstall "$uninstalled_version" no "native-${manager}"
	fi
	if [ "$manager" = systemd ]; then
		service_control daemon-reload
	fi
	commit_transaction
	if [ "$purge_data" = yes ]; then
		rm -rf -- "$confdir" "$state_dir"
		printf 'removed programs, units, configuration, and state\n'
	else
		printf 'removed programs and units; preserved %s and %s\n' "$confdir" "$state_dir"
	fi
}
