package moderate

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/mymmrac/telego"
)

type setupLookup struct {
	chats        map[int64]*telego.ChatFullInfo
	chatErrors   map[int64]error
	members      map[int64]telego.ChatMember
	memberErrors map[int64]error
	sendErrors   map[int64]error
	sentTo       []int64
}

func (l *setupLookup) GetChat(_ context.Context, params *telego.GetChatParams) (*telego.ChatFullInfo, error) {
	chatID := params.ChatID.ID
	return l.chats[chatID], l.chatErrors[chatID]
}

func (l *setupLookup) GetChatMember(_ context.Context, params *telego.GetChatMemberParams) (telego.ChatMember, error) {
	chatID := params.ChatID.ID
	return l.members[chatID], l.memberErrors[chatID]
}

func (l *setupLookup) SendMessage(_ context.Context, params *telego.SendMessageParams) (*telego.Message, error) {
	chatID := params.ChatID.ID
	l.sentTo = append(l.sentTo, chatID)
	if err := l.sendErrors[chatID]; err != nil {
		return nil, err
	}
	return &telego.Message{MessageID: len(l.sentTo)}, nil
}

func completeSetupAdministrator(selfID int64) telego.ChatMember {
	return &telego.ChatMemberAdministrator{
		Status:             telego.MemberStatusAdministrator,
		User:               telego.User{ID: selfID},
		CanInviteUsers:     true,
		CanRestrictMembers: true,
		CanDeleteMessages:  true,
	}
}

func newRegisteredSetupService(t *testing.T, groupID, registrantID, adminLogID int64) *Service {
	t.Helper()
	cfg := &settings.Config{AdminLogChatID: adminLogID, Lang: "en"}
	directory := t.TempDir()
	baseline, err := settings.LoadBaseline(filepath.Join(directory, "missing-config.json"), cfg)
	if err != nil {
		t.Fatalf("load settings baseline: %v", err)
	}
	store, err := settings.NewStore(filepath.Join(directory, "settings.json"), baseline, nil)
	if err != nil {
		t.Fatalf("create settings store: %v", err)
	}
	registration := store.Registrations()
	registration.RegisteredGroups = []settings.RegisteredGroup{{
		ID: groupID, RegisteredBy: registrantID, Title: "Registered Group",
	}}
	if _, err := store.CommitRegistrations(registration.Revision, registration); err != nil {
		t.Fatalf("register test group: %v", err)
	}
	if adminLogID != 0 {
		group, ok := store.Settings(groupID)
		if !ok {
			t.Fatalf("registered group %d missing from settings", groupID)
		}
		overrides := group.Overrides()
		overrides.AdminLogChatID = &adminLogID
		if _, err := store.Update(groupID, group.Revision(), overrides); err != nil {
			t.Fatalf("set admin log target: %v", err)
		}
	}
	service, err := New(store, newFakeMod(), cfg, nil)
	if err != nil {
		t.Fatalf("create moderation service: %v", err)
	}
	return service
}

func requireSetupTargets(t *testing.T, got []int64, want ...int64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("setup delivery targets = %v, want %v; registrant must be first and zero or repeated targets must not be tried", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("setup delivery targets = %v, want %v; registrant must be first and zero or repeated targets must not be tried", got, want)
		}
	}
}

func TestGroupSetupReportsUnreachableGroupAndAcceptsReachableGroup(t *testing.T) {
	const (
		groupID = int64(-1009000000801)
		selfID  = int64(9001)
	)
	cfg := &settings.Config{
		Groups:   []settings.GroupConfig{{ID: groupID}},
		GroupIDs: []int64{groupID},
		Lang:     "en",
	}
	service := newTestService(t, cfg, newFakeMod(), "")
	lookup := &setupLookup{
		chats: map[int64]*telego.ChatFullInfo{
			groupID: {ID: groupID, Type: telego.ChatTypeSupergroup, Title: "Readable Group"},
		},
		chatErrors: map[int64]error{groupID: errors.New("getChat unavailable")},
		members:    map[int64]telego.ChatMember{groupID: completeSetupAdministrator(selfID)},
	}

	report := service.CheckGroupSetup(context.Background(), lookup, selfID, groupID)
	wantMissing := i18n.Messages.Moderate.Setup.GroupAccess.For(i18n.LangEN)
	if report.Ready || !strings.Contains(report.Text, wantMissing) {
		t.Fatalf("unreachable group report = ready %t text %q, want missing group access %q", report.Ready, report.Text, wantMissing)
	}

	delete(lookup.chatErrors, groupID)
	report = service.CheckGroupSetup(context.Background(), lookup, selfID, groupID)
	if !report.Ready {
		t.Fatalf("reachable group report = ready %t text %q, want ready", report.Ready, report.Text)
	}
}

func TestUnreadySetupReportTriesRegistrantFirstAndEachValidTargetOnce(t *testing.T) {
	const (
		groupID      = int64(-1009000000802)
		adminLogID   = int64(-1009000000803)
		registrantID = int64(9002)
		selfID       = int64(9003)
	)
	newLookup := func() *setupLookup {
		member := completeSetupAdministrator(selfID).(*telego.ChatMemberAdministrator)
		member.CanDeleteMessages = false
		return &setupLookup{
			chats:   map[int64]*telego.ChatFullInfo{groupID: {ID: groupID, Type: telego.ChatTypeSupergroup}},
			members: map[int64]telego.ChatMember{groupID: member},
		}
	}

	t.Run("registrant receives the first attempt", func(t *testing.T) {
		service := newRegisteredSetupService(t, groupID, registrantID, adminLogID)
		lookup := newLookup()
		service.LogGroupSetup(context.Background(), lookup, selfID, groupID)
		requireSetupTargets(t, lookup.sentTo, registrantID)
	})

	t.Run("duplicate fallback target is tried once", func(t *testing.T) {
		service := newRegisteredSetupService(t, groupID, registrantID, groupID)
		lookup := newLookup()
		lookup.sendErrors = map[int64]error{
			registrantID: errors.New("registrant DM unavailable"),
			groupID:      errors.New("group delivery unavailable"),
		}
		service.LogGroupSetup(context.Background(), lookup, selfID, groupID)
		requireSetupTargets(t, lookup.sentTo, registrantID, groupID)
	})

	t.Run("absent optional targets are skipped", func(t *testing.T) {
		cfg := &settings.Config{
			Groups:   []settings.GroupConfig{{ID: groupID}},
			GroupIDs: []int64{groupID},
			Lang:     "en",
		}
		service := newTestService(t, cfg, newFakeMod(), "")
		lookup := newLookup()
		lookup.sendErrors = map[int64]error{0: errors.New("zero is not a Telegram target")}
		service.LogGroupSetup(context.Background(), lookup, selfID, groupID)
		requireSetupTargets(t, lookup.sentTo, groupID)
	})
}

func TestGroupSetupReportNamesReadableGroupByTelegramTitle(t *testing.T) {
	const (
		groupID = int64(-1009000000804)
		selfID  = int64(9004)
		title   = "Readable Moderation Group"
	)
	cfg := &settings.Config{
		Groups:   []settings.GroupConfig{{ID: groupID}},
		GroupIDs: []int64{groupID},
		Lang:     "en",
	}
	service := newTestService(t, cfg, newFakeMod(), "")
	lookup := &setupLookup{
		chats: map[int64]*telego.ChatFullInfo{
			groupID: {ID: groupID, Type: telego.ChatTypeSupergroup, Title: title},
		},
		members: map[int64]telego.ChatMember{groupID: completeSetupAdministrator(selfID)},
	}

	report := service.CheckGroupSetup(context.Background(), lookup, selfID, groupID)
	want := i18n.Messages.Moderate.Setup.Ready.Render(i18n.LangEN, title, groupID)
	if !report.Ready || report.Text != want {
		t.Fatalf("readable group report = ready %t text %q, want Telegram title in %q", report.Ready, report.Text, want)
	}
}

func TestMuteDeletesOffendingMessageAfterRestrictionSucceeds(t *testing.T) {
	const groupID = int64(-1009000000805)
	telegram := newFakeMod()
	telegram.memberByID = map[int64]telego.ChatMember{
		7: &telego.ChatMemberAdministrator{},
		8: &telego.ChatMemberMember{},
	}
	service := newTestService(t, &settings.Config{
		GroupIDs:         []int64{groupID},
		Groups:           []settings.GroupConfig{{ID: groupID}},
		Lang:             "en",
		MuteSeconds:      3600,
		NotifyTTLSeconds: -1,
	}, telegram, "")
	message := moderationCommand(groupID, "/mute")

	runFakeHandler(t, newAPITestBot(t, telegram), service.OnMute, telego.Update{Message: message})
	if telegram.mutes != 1 {
		t.Fatalf("mute calls = %d, want one successful restriction before cleanup", telegram.mutes)
	}
	if got := telegram.deletedMessageIDs; len(got) != 2 ||
		got[0] != message.ReplyToMessage.MessageID || got[1] != message.MessageID {
		t.Fatalf("successful mute deleted message IDs = %v, want offending message %d then command %d",
			got, message.ReplyToMessage.MessageID, message.MessageID)
	}
}
