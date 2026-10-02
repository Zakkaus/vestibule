# Native deployment regression cases sourced by test-install.sh.

case_native_deployment() {
	new_sandbox native-deployment
	run_success "${sandbox}/install.out" v1.0.0
	deployment_record=${test_root}/etc/vestibule/deployment.env
	state=${test_root}/var/lib/vestibule
	printf 'v2.0.0\n' > "${state}/replacement-request"
	if ! FETCH_LOG=$fetch_log SYSTEMCTL_LOG=$systemctl_log FIXTURE_RELEASES=$fixtures \
		VESTIBULE_FETCH=$fake_fetch VESTIBULE_SYSTEMCTL=$fake_systemctl \
		VESTIBULE_ROOT=$test_root VESTIBULE_REPLACE_HEALTH_COMMAND=true \
		TMPDIR=${sandbox}/tmp "${test_root}/usr/local/libexec/vestibule-replace" \
		> "${sandbox}/replacement.out" 2>&1; then
		fail "installed native replacement failed: $(cat "${sandbox}/replacement.out")"
	fi
	assert_line 'status=applied' "${state}/replacement-result.env"
	assert_line 'v2.0.0' "${test_root}/etc/vestibule/release-version"
	assert_same "${fixtures}/v2.0.0/vestibule-linux-${arch}" "${test_root}/usr/local/bin/vestibule"
	assert_absent "${state}/replacement-request"
	assert_mode "$deployment_record" 600

	# An upgrade repairs legacy installations; a failed repair must remain absent.
	rm -f "$deployment_record"
	fail_systemctl=restart
	if run_installer v2.0.0 > "${sandbox}/failed-upgrade.out" 2>&1; then
		fail "native upgrade succeeded despite a failed restart"
	fi
	assert_absent "$deployment_record"
	fail_systemctl=
	run_success "${sandbox}/legacy-upgrade.out" v2.0.0
	assert_line 'deployment=native' "$deployment_record"
	fail_systemctl=restart
	if run_installer v2.0.0 > "${sandbox}/failed-existing.out" 2>&1; then
		fail "native upgrade succeeded despite a failed restart"
	fi
	assert_line 'deployment=native' "$deployment_record"
	fail_systemctl=
	run_success "${sandbox}/uninstall.out" --uninstall --keep-data
	assert_absent "$deployment_record"
	assert_file "${state}/claim.json"
	pass "native installation enables host replacement, repairs legacy records transactionally, and removes its record on uninstall"
}
