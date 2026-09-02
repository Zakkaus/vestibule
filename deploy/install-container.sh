# Container lifecycle actions for install.sh.

container_control() {
	"$container_runtime" compose --project-directory "$confdir" --env-file "$container_env" \
		-f "$compose_file" "$@"
}

detect_container_runtime() {
	if [ -n "$container_runtime_override" ]; then
		command -v "$container_runtime_override" >/dev/null 2>&1 || fail "VESTIBULE_DOCKER is not executable: $container_runtime_override"
		container_runtime=$container_runtime_override
	elif command -v docker >/dev/null 2>&1; then
		container_runtime=$(command -v docker)
	else
		fail "container deployment requires Docker with the Compose plugin"
	fi
	"$container_runtime" compose version >/dev/null 2>&1 || fail "container deployment requires the Docker Compose plugin"
}

ensure_container_state_directory() {
	directory=$1
	if [ -e "$directory" ]; then
		[ -d "$directory" ] || fail "$directory exists and is not a directory"
		if [ -z "$root" ]; then
			owner=$(stat -c '%u:%g' "$directory")
			[ "$owner" = 65532:65532 ] || fail "$directory is not owned by the container application user; do not change existing state ownership implicitly"
		fi
		return
	fi
	ensure_directory "$directory"
	[ -n "$root" ] || chown 65532:65532 "$directory"
}

require_bot_api_credentials() {
	[ -f "$bot_api_env" ] || fail "container deployment requires $bot_api_env with TELEGRAM_API_ID and TELEGRAM_API_HASH; the upstream Bot API requires both and this installer does not invent credentials"
	api_id_count=$(awk -F= '$1 == "TELEGRAM_API_ID" && length($2) > 0 { count++ } END { print count + 0 }' "$bot_api_env")
	api_hash_count=$(awk -F= '$1 == "TELEGRAM_API_HASH" && length($2) > 0 { count++ } END { print count + 0 }' "$bot_api_env")
	[ "$api_id_count" = 1 ] && [ "$api_hash_count" = 1 ] || fail "$bot_api_env must contain exactly one nonempty TELEGRAM_API_ID and TELEGRAM_API_HASH"
}

ensure_container_config() {
	if [ ! -e "$config_file" ]; then
		printf '{}\n' > "${work}/config.json"
		txn_replace "${work}/config.json" 600 "$config_file"
		[ -n "$root" ] || chown 65532:65532 "$config_file"
		return
	fi
	[ -f "$config_file" ] || fail "$config_file exists and is not a regular file"
	if [ -z "$root" ]; then
		owner=$(stat -c '%u:%g' "$config_file")
		[ "$owner" = 65532:65532 ] || fail "$config_file is not owned by the container application user; do not change existing configuration ownership implicitly"
	fi
}

write_container_environment() {
	if [ "$operation" = upgrade ]; then
		awk -F= -v app="ghcr.io/zakkaus/vestibule:${version}" \
			-v bot_api="ghcr.io/zakkaus/vestibule-bot-api:${version}" '
			$1 == "VESTIBULE_APP_IMAGE" {
				if (app_seen++) exit 1
				print "VESTIBULE_APP_IMAGE=" app
				next
			}
			$1 == "VESTIBULE_BOT_API_IMAGE" {
				if (bot_api_seen++) exit 1
				print "VESTIBULE_BOT_API_IMAGE=" bot_api
				next
			}
			{ print }
			END { if (app_seen != 1 || bot_api_seen != 1) exit 1 }
		' "$container_env" > "${work}/container.env" \
			|| fail "existing container environment record is malformed"
	else
		database_password=$(random_setup_token)
		cat > "${work}/container.env" <<EOF
VESTIBULE_APP_IMAGE=ghcr.io/zakkaus/vestibule:${version}
VESTIBULE_STATE_DIRECTORY=${state_dir}
VESTIBULE_CONFIG_PATH=${config_file}
VESTIBULE_BOT_API_IMAGE=ghcr.io/zakkaus/vestibule-bot-api:${version}
VESTIBULE_BOT_API_STATE_DIRECTORY=${bot_api_state_dir}
VESTIBULE_POSTGRES_PASSWORD=${database_password}
VESTIBULE_DATABASE_URI=postgres://vestibule:${database_password}@database:5432/vestibule?sslmode=disable
EOF
	fi
	txn_replace "${work}/container.env" 600 "$container_env"
}

stage_container_previous_release() {
	cp -p -- "$container_env" "${work}/retained-container.env"
	cp -p -- "$current_manifest" "${work}/retained-manifest"
	cp -p -- "$current_version_file" "${work}/retained-version"
	txn_replace "${work}/retained-container.env" 600 "$previous_container_env"
	txn_replace "${work}/retained-manifest" 644 "$previous_manifest"
	txn_replace "${work}/retained-version" 644 "$previous_version_file"
}

container_install_or_upgrade() {
	require_mutation_access
	make_workdir
	detect_fetcher
	detect_service_manager
	detect_container_runtime
	operation=install
	if [ -f "$deployment_file" ]; then
		operation=upgrade
	fi
	if [ "$action" = upgrade ] && [ "$operation" != upgrade ]; then
		fail "--upgrade requires an existing container deployment"
	fi
	require_bot_api_credentials
	resolve_version
	printf '%s container deployment %s\n' "$operation" "$version"
	fetch_release_manifest
	if [ "$operation" = upgrade ]; then
		assess_upgrade_rollback
	fi
	fetch_release_support
	begin_transaction
	ensure_directory "$confdir"
	ensure_bot_env
	ensure_container_config
	ensure_container_state_directory "$state_dir"
	ensure_container_state_directory "$bot_api_state_dir"
	if [ "$operation" = upgrade ]; then
		[ -f "$container_env" ] || fail "existing container deployment has no environment record"
		[ -f "$current_version_file" ] || fail "existing container deployment has no stored release version"
		stage_container_previous_release
	fi
	txn_replace "${work}/${name}-schema-manifest" 644 "$current_manifest"
	printf '%s\n' "$version" > "${work}/release-version"
	txn_replace "${work}/release-version" 644 "$current_version_file"
	write_container_environment
	printf 'deployment=container\n' > "${work}/deployment.env"
	txn_replace "${work}/deployment.env" 600 "$deployment_file"
	txn_replace "${work}/compose.yaml" 644 "$compose_file"
	install_support_files
	container_changed=yes
	container_control pull app bot-api database
	container_control up -d
	write_replacement_unit_state
	if [ "$operation" = upgrade ]; then
		rollback_available=yes
	else
		rollback_available=no
	fi
	write_result "$operation" "$version" "$rollback_available" container
	commit_transaction
	printf '%s container %s complete\n' "$name" "$operation"
	printf 'result file: %s\n' "$result_file"
}

container_rollback_release() {
	require_mutation_access
	for required in "$container_env" "$previous_container_env" "$current_manifest" "$previous_manifest" \
		"$current_version_file" "$previous_version_file" "$compose_file"; do
		[ -f "$required" ] || fail "cannot roll back; retained file is missing: $required"
	done
	make_workdir
	detect_service_manager
	detect_container_runtime
	check_manual_rollback
	begin_transaction
	swap_release_file "$container_env" "$previous_container_env" 600 container-env
	swap_release_file "$current_manifest" "$previous_manifest" 644 manifest
	swap_release_file "$current_version_file" "$previous_version_file" 644 version
	container_changed=yes
	container_control up -d --no-deps app
	rolled_back_version=$(sed -n '1p' "$current_version_file")
	write_result rollback "$rolled_back_version" yes container
	commit_transaction
	printf '%s container rolled back to %s\n' "$name" "$rolled_back_version"
}

container_show_status() {
	if [ ! -f "$deployment_file" ]; then
		printf 'installed=no\n'
		return 1
	fi
	detect_service_manager
	detect_container_runtime
	installed_version=unknown
	[ ! -f "$current_version_file" ] || installed_version=$(sed -n '1p' "$current_version_file")
	if [ -f "$previous_container_env" ] && [ -f "$previous_manifest" ]; then
		rollback_available=yes
	else
		rollback_available=no
	fi
	printf 'installed=yes\nversion=%s\ndeployment=container\nrollback_available=%s\n' \
		"$installed_version" "$rollback_available"
	if [ -f "$replacement_unit_state" ] && grep -Fqx 'available=yes' "$replacement_unit_state"; then
		printf 'replacement_unit=available\n'
	else
		printf 'replacement_unit=unavailable\n'
	fi
	container_control ps
	last_operation=$(result_value operation)
	[ -z "$last_operation" ] || printf 'last_operation=%s\n' "$last_operation"
	printf 'result_file=%s\n' "$result_file"
}

restore_container_runtime() {
	[ "$container_changed" = yes ] || return 0
	if [ "$had_container_before" = yes ]; then
		container_control up -d >/dev/null 2>&1 || :
	else
		container_control down >/dev/null 2>&1 || :
	fi
}

container_uninstall_release() {
	require_mutation_access
	make_workdir
	detect_service_manager
	detect_container_runtime
	choose_purge_data
	begin_transaction
	if [ "$purge_data" = yes ]; then
		container_control down -v
	else
		container_control down
	fi
	container_changed=yes
	if [ "$manager" = systemd ] && [ -e "$replacement_path" ]; then
		service_control disable --now "$replacement_path_name"
		replacement_unit_changed=yes
	fi
	for installed_file in "$compose_file" "$container_env" "$previous_container_env" "$deployment_file" \
		"$current_manifest" "$previous_manifest" "$current_version_file" "$previous_version_file" \
		"$managed_installer" "$managed_common" "$managed_native" "$managed_container" \
		"$replacement_runner" "$replacement_service" "$replacement_path" "$replacement_unit_state"; do
		txn_remove "$installed_file"
	done
	if [ -d "$state_dir" ]; then
		uninstalled_version=$(result_value version)
		[ -n "$uninstalled_version" ] || uninstalled_version=unknown
		write_result uninstall "$uninstalled_version" no container
	fi
	if [ "$manager" = systemd ]; then
		service_control daemon-reload
	fi
	commit_transaction
	if [ "$purge_data" = yes ]; then
		rm -rf -- "$confdir" "$state_dir" "$bot_api_state_dir"
		printf 'removed containers, programs, configuration, and state\n'
	else
		printf 'removed containers and programs; preserved %s, %s, and %s\n' "$confdir" "$state_dir" "$bot_api_state_dir"
	fi
}
