package i18n

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestCatalogueFormatsAcceptTheirRenderArguments(t *testing.T) {
	contracts := parseRenderArgumentContracts(t)
	seen := make(map[string]bool, len(contracts))
	visitCatalog(reflect.ValueOf(Messages), "Messages", func(path string, value reflect.Value) {
		if value.Type() != formatType {
			return
		}
		subsystem, key := catalogPath(path)
		contractKey := subsystem + "." + key
		want, ok := contracts[contractKey]
		if !ok {
			t.Errorf("%s has no Render argument contract; callers can change without a catalogue check", catalogEntry(path, LangEN))
			return
		}
		seen[contractKey] = true
		format := value.Interface().(Format)
		for _, locale := range Languages() {
			location := catalogEntry(path, locale)
			got, err := formatArgumentContract(format.value(locale))
			if err != nil {
				t.Errorf("%s has an invalid Render argument contract: %v; users would see missing values or fmt diagnostics", location, err)
				continue
			}
			if got != want {
				t.Errorf("%s Render arguments = %q, want %q; users would see missing values or fmt diagnostics", location, got, want)
				continue
			}
			arguments, markers := renderArguments(t, want)
			rendered := format.Render(locale, arguments...)
			if strings.Contains(rendered, "%!") {
				t.Errorf("%s renders fmt diagnostic %q instead of real values", location, rendered)
				continue
			}
			for _, marker := range markers {
				if !strings.Contains(rendered, marker) {
					t.Errorf("%s drops Render argument %q; users would receive an incomplete message", location, marker)
				}
			}
		}
	})
	for key := range contracts {
		if !seen[key] {
			t.Errorf("Render argument contract %s has no Format catalogue entry", key)
		}
	}
}

func parseRenderArgumentContracts(t *testing.T) map[string]string {
	t.Helper()
	contracts := make(map[string]string)
	for lineNumber, line := range strings.Split(strings.TrimSpace(renderArgumentContracts), "\n") {
		key, contract, ok := strings.Cut(line, "=")
		if !ok || key == "" || contract == "" {
			t.Fatalf("render argument contract line %d is malformed: %q", lineNumber+1, line)
		}
		if _, duplicate := contracts[key]; duplicate {
			t.Fatalf("render argument contract %s is duplicated", key)
		}
		contracts[key] = contract
	}
	return contracts
}

func formatArgumentContract(format string) (string, error) {
	placeholders, err := indexedPlaceholders(format)
	if err != nil {
		return "", err
	}
	verbs := make(map[int]byte)
	maximum := 0
	for _, placeholder := range placeholders {
		indexText, verbText, ok := strings.Cut(placeholder, ":")
		index, conversionErr := strconv.Atoi(indexText)
		if !ok || conversionErr != nil || len(verbText) != 1 {
			return "", fmt.Errorf("invalid parsed placeholder %q", placeholder)
		}
		if previous, exists := verbs[index]; exists && previous != verbText[0] {
			return "", fmt.Errorf("argument %d uses both %%%c and %%%c", index, previous, verbText[0])
		}
		verbs[index] = verbText[0]
		maximum = max(maximum, index)
	}
	parts := make([]string, maximum)
	for index := 1; index <= maximum; index++ {
		verb, ok := verbs[index]
		if !ok {
			return "", fmt.Errorf("argument index %d is missing", index)
		}
		parts[index-1] = fmt.Sprintf("%d:%c", index, verb)
	}
	return strings.Join(parts, ","), nil
}

func renderArguments(t *testing.T, contract string) ([]any, []string) {
	t.Helper()
	parts := strings.Split(contract, ",")
	arguments := make([]any, len(parts))
	markers := make([]string, len(parts))
	for position, part := range parts {
		indexText, verbText, ok := strings.Cut(part, ":")
		index, err := strconv.Atoi(indexText)
		if !ok || err != nil || index != position+1 || len(verbText) != 1 {
			t.Fatalf("malformed Render argument contract %q", contract)
		}
		switch verbText[0] {
		case 'd':
			value := 91000 + index
			arguments[position] = value
			markers[position] = strconv.Itoa(value)
		case 's', 'v':
			value := "render-argument-" + strconv.Itoa(index)
			arguments[position] = value
			markers[position] = value
		default:
			t.Fatalf("Render argument contract %q uses unsupported verb %%%s", contract, verbText)
		}
	}
	return arguments, markers
}

const renderArgumentContracts = `
bot.direct_message.auto_reply=1:d,2:s
bot.lifecycle.unauthorized_chat=1:s,2:d,3:s
bot.menu.admin.warn=1:d
bot.registration.enrollment_link=1:d,2:s
bot.registration.group_registered=1:s
bot.registration.group_unregistered=1:s
bot.registration.registration_pending=1:s
lookup_content.bbs.heading=1:s
lookup_content.bug.details.product_component=1:s
lookup_content.bug.details.severity=1:s
lookup_content.bug.details.status=1:s
lookup_content.bug.heading=1:s,2:s,3:s
lookup_content.bug.not_found=1:s
lookup_content.bug.unavailable=1:s,2:s
lookup_content.news.search_heading=1:s
lookup_content.transport.private_rate_limited=1:d
lookup_content.wiki.heading=1:s
lookup_content.wiki.sources_unavailable=1:s
lookup_distros.armpkgs.available=1:s,2:s
lookup_distros.armpkgs.development_suite=1:s
lookup_distros.armpkgs.fedora_rawhide=1:s
lookup_distros.armpkgs.heading=1:s
lookup_distros.armpkgs.row=1:s,2:s,3:s
lookup_distros.armpkgs.stable_only=1:s
lookup_distros.armpkgs.stable_testing=1:s,2:s
lookup_distros.armpkgs.testing_only=1:s
lookup_distros.cve.heading=1:s,2:s
lookup_distros.cve.not_found=1:s
lookup_distros.cve.published=1:s,2:s
lookup_distros.cve.severity=1:s,2:s
lookup_distros.kernel.row=1:s,2:s,3:s
lookup_distros.man.heading=1:s,2:s
lookup_distros.man.not_found=1:s
lookup_distros.man.synopsis=1:s
lookup_distros.pkgs.alternatives=1:s
lookup_distros.pkgs.closest_match=1:s
lookup_distros.pkgs.heading=1:s,2:s
lookup_distros.pkgs.no_supported_distro=1:s
lookup_distros.pkgs.plain_heading=1:s
lookup_distros.pkgs.plain_row=1:s,2:s,3:s
lookup_distros.pkgs.release_role=1:s
lookup_distros.pkgs.repology_not_found=1:s
lookup_distros.pkgs.repology_unavailable=1:s
lookup_distros.pkgs.rich_alternatives=1:d,2:s
lookup_distros.pkgs.rich_row=1:s,2:s,3:s
lookup_distros.repology.heading=1:s,2:s
lookup_distros.repology.more=1:d
lookup_distros.repology.not_found=1:s
lookup_distros.repology.row=1:s,2:s
lookup_packages.arm.heading=1:s,2:s
lookup_packages.arm.not_found=1:s
lookup_packages.arm.stable_only=1:s
lookup_packages.arm.stable_testing=1:s,2:s
lookup_packages.arm.testing_only=1:s
lookup_packages.pkg.overlay_count=1:d
lookup_packages.pkg.results_heading=1:s
lookup_packages.pkg.unavailable=1:s
lookup_packages.source.overlay=1:s
lookup_packages.source.partial_results=1:s
lookup_packages.use.also_in_overlay=1:s
lookup_packages.use.count=1:d
lookup_packages.use.info_unavailable=1:s
lookup_packages.use.not_found=1:s
lookup_packages.use.partial_matches=1:s
lookup_packages.use.source_label=1:s
lookup_packages.use.truncated_count=1:d
lookup_packages.use.unavailable=1:s,2:s
lookup_packages.use.version_latest=1:s
lookup_packages.use.version_stable=1:s
lookup_packages.use.version_stable_latest=1:s,2:s
moderate.antispam.allowed=1:d
moderate.antispam.allowed_unban_failed=1:d
moderate.antispam.removed=1:d
moderate.antispam.sender_ban_failed_alert=1:s,2:d,3:d
moderate.antispam.sender_banned_alert=1:s,2:d,3:d,4:d
moderate.ban.action=1:s,2:s
moderate.ban.alert=1:s,2:s,3:d,4:d,5:s,6:s
moderate.ban.applied=1:s,2:s,3:d,4:s
moderate.ban.failure_alert=1:s,2:d,3:d,4:s,5:s
moderate.ban_time.current=1:s,2:s,3:s
moderate.ban_time.set=1:s,2:s
moderate.common.command_admin_only=1:s
moderate.common.reply_usage=1:s
moderate.duration.days=1:d
moderate.duration.hours=1:d
moderate.duration.minutes=1:d
moderate.duration.seconds=1:d
moderate.duration.status=1:s,2:s
moderate.mute.alert=1:s,2:d,3:d,4:s,5:s
moderate.mute.applied=1:s,2:d,3:s,4:s
moderate.mute.unmuted=1:s,2:d,3:s
moderate.mute.usage=1:s
moderate.setup.channel_admin=1:s,2:d
moderate.setup.missing_header=1:s,2:d
moderate.setup.ready=1:s,2:d
moderate.warning.cleared=1:s,2:d,3:s
moderate.warning.issued=1:s,2:d,3:d,4:d,5:s
moderate.warning.kick_alert=1:d,2:d,3:s,4:s
moderate.warning.limit_kick_alert=1:s,2:d,3:s
moderate.warning.limit_reached=1:s,2:d,3:s,4:s
panel.auto_delete.current_enabled=1:d
panel.auto_delete.enabled=1:d
panel.auto_delete.set=1:d
panel.help.admin=1:d
panel.help.direct_message_note=1:d
panel.help.group_state=1:s
panel.settings.screen.channel=1:d,2:d,3:s,4:s
panel.settings.screen.confirm=1:s
panel.settings.screen.content=1:d,2:d,3:s,4:s,5:s
panel.settings.screen.fallback_bank=1:d,2:s,3:d,4:s
panel.settings.screen.fallback_detail=1:s,2:s
panel.settings.screen.group_home=1:s,2:d,3:d,4:s,5:s,6:s,7:s,8:d,9:d,10:s
panel.settings.screen.groups=1:d,2:s
panel.settings.screen.input=1:s
panel.settings.screen.list=1:s,2:d,3:d,4:s
panel.settings.screen.lists=1:d,2:d,3:d,4:d
panel.settings.screen.moderation=1:d,2:s,3:s,4:s,5:s,6:s
panel.settings.screen.quiz_bank=1:d,2:d,3:s
panel.settings.screen.quiz_detail=1:s,2:s,3:s
panel.settings.screen.runtime=1:d,2:s,3:s,4:s,5:s,6:s,7:s,8:s,9:s
panel.settings.screen.verification=1:d,2:s,3:s,4:s,5:s,6:s
panel.settings.value.answer_item=1:d,2:s
panel.settings.value.group_button=1:s,2:d
panel.settings.value.id_item=1:d
panel.settings.value.minutes=1:d
panel.settings.value.option_item=1:d,2:s
panel.settings.value.question_item=1:d,2:s
panel.settings.value.seconds=1:d
panel.settings.value.sourced=1:s,2:s
panel.status.ping=1:s,2:s,3:s
panel.status.stats=1:s,2:d,3:d,4:s,5:s
panel.verification_mode.auto_set=1:s
panel.verification_mode.current=1:s,2:s
panel.verification_mode.set=1:s
verification.admin.agent_caught=1:d,2:d,3:s,4:d
verification.admin.agent_stats=1:d,2:s
verification.admin.approve_failed=1:d,2:d,3:v
verification.admin.approve_failed_held=1:d,2:d,3:v
verification.admin.auto_ban_failed=1:d,2:d,3:d,4:v
verification.admin.auto_banned=1:d,2:d,3:d,4:s
verification.admin.ban_failed=1:d,2:d,3:v
verification.admin.ban_failed_held=1:d,2:d,3:v
verification.admin.banning=1:s
verification.admin.banning_held=1:s
verification.admin.challenge_post_failed=1:d,2:d,3:v
verification.admin.challenge_post_failed_held=1:d,2:d,3:v
verification.admin.channel_access_failed=1:d,2:s
verification.admin.decline_failed=1:d,2:d,3:v
verification.admin.decline_failed_held=1:d,2:d,3:v
verification.admin.outage_backlog=1:d
verification.admin.pending_cap=1:d,2:d,3:d
verification.admin.pending_cap_held=1:d,2:d,3:d
verification.admin.settings_degraded=1:s
verification.admin.trusted_bypass_failed=1:d,2:d,3:d,4:v
verification.challenge.agent_trap=1:s
verification.challenge.fallback_intro=1:s,2:d
verification.challenge.fallback_wrong=1:d
verification.challenge.fallback_wrong_held=1:d
verification.challenge.kernel_prompt=1:s,2:d
verification.challenge.kernel_prompt_held=1:s,2:d
verification.challenge.kernel_wrong=1:d
verification.challenge.kernel_wrong_held=1:d
verification.challenge.quiz_prompt=1:s
verification.channel.first=1:s
verification.channel.follow_button=1:s
verification.channel.follow_prompt=1:s
verification.channel.not_followed_yet=1:s
verification.duration.days=1:d
verification.duration.hours=1:d
verification.duration.minutes=1:d
verification.duration.seconds=1:d
verification.group.body=1:s,2:s,3:d,4:s
verification.group.body_invited=1:s,2:s,3:d,4:s
verification.group.body_joined=1:s,2:s,3:d,4:s
verification.group.body_recovered=1:s,2:s,3:s,4:s
verification.group.channel_hint=1:s
verification.group.link_text=1:s
verification.held.ai_caught=1:d
verification.held.cooldown_active=1:d
verification.held.timeout_banned=1:s
verification.held.timeout_retry=1:d
verification.held.wrong_banned=1:s
verification.held.wrong_retry=1:d
verification.recovery.outage_hours=1:d
verification.recovery.outage_minutes=1:d
verification.recovery.outage_seconds=1:d
verification.recovery.renotify=1:s
verification.result.ai_caught=1:d
verification.result.cooldown_active=1:d
verification.result.timeout_banned=1:s
verification.result.timeout_retry=1:d
verification.result.wrong_banned=1:s
verification.result.wrong_retry=1:d
`
