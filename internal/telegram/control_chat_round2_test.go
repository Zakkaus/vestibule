package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/mymmrac/telego"
	ta "github.com/mymmrac/telego/telegoapi"
)

const controlRound2Chat int64 = -1009000000701
const controlRound2Group int64 = -1009000000702
const controlRound2Actor int64 = 42

type controlChatCaller struct {
	*registrationCaller
	chatType string
	chatErr  error
}

func (c *controlChatCaller) Call(ctx context.Context, endpoint string, data *ta.RequestData) (*ta.Response, error) {
	if !strings.HasSuffix(endpoint, "/getChat") {
		return c.registrationCaller.Call(ctx, endpoint, data)
	}
	if c.chatErr != nil {
		return nil, c.chatErr
	}
	var params struct {
		ChatID int64 `json:"chat_id"`
	}
	if err := json.Unmarshal(data.BodyRaw, &params); err != nil {
		return nil, err
	}
	return registrationAPIResponse(&telego.ChatFullInfo{ID: params.ChatID, Type: c.chatType})
}

func controlAssignmentStore(t *testing.T, caller *controlChatCaller) *settings.Store {
	t.Helper()
	cfg := &settings.Config{GroupIDs: []int64{controlRound2Group}}
	baseline, err := settings.LoadBaseline("", cfg)
	if err != nil {
		t.Fatal(err)
	}
	store, err := settings.NewStore("", baseline, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	newRegistrationService(context.Background(), newRegistrationBot(t, caller), store, cfg, "test_bot", testBotID, nil, nil, nil)
	return store
}

func TestControlChatRound2RequiresControlAdministrator(t *testing.T) {
	for _, tc := range []struct {
		name      string
		actor     telego.ChatMember
		lookupErr error
		allowed   bool
	}{
		{"owner", &telego.ChatMemberOwner{Status: telego.MemberStatusCreator, User: telego.User{ID: controlRound2Actor}}, nil, true},
		{"administrator", adminMember(controlRound2Actor), nil, true},
		{"member", plainMember(controlRound2Actor), nil, false},
		{"left", &telego.ChatMemberLeft{Status: telego.MemberStatusLeft}, nil, false},
		{"lookup unavailable", nil, errors.New("unavailable"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &controlChatCaller{registrationCaller: &registrationCaller{
				members:      map[[2]int64]telego.ChatMember{{controlRound2Chat, testBotID}: plainMember(testBotID), {controlRound2Chat, controlRound2Actor}: tc.actor},
				memberErrors: map[[2]int64]error{{controlRound2Chat, controlRound2Actor}: tc.lookupErr}}, chatType: telego.ChatTypeSupergroup}
			store := controlAssignmentStore(t, caller)
			assertControlAssignment(t, store, tc.allowed)
		})
	}
}

func TestControlChatRound2RejectsUnsupportedChatTypes(t *testing.T) {
	for _, tc := range []struct {
		chatType string
		allowed  bool
	}{
		{telego.ChatTypeGroup, true}, {telego.ChatTypeSupergroup, true},
		{telego.ChatTypeChannel, false}, {telego.ChatTypePrivate, false}, {"", false},
	} {
		t.Run(tc.chatType, func(t *testing.T) {
			caller := &controlChatCaller{registrationCaller: &registrationCaller{
				members: map[[2]int64]telego.ChatMember{{controlRound2Chat, testBotID}: plainMember(testBotID), {controlRound2Chat, controlRound2Actor}: adminMember(controlRound2Actor)}}, chatType: tc.chatType}
			store := controlAssignmentStore(t, caller)
			assertControlAssignment(t, store, tc.allowed)
		})
	}
	t.Run("chat lookup unavailable", func(t *testing.T) {
		caller := &controlChatCaller{registrationCaller: &registrationCaller{
			members: map[[2]int64]telego.ChatMember{{controlRound2Chat, testBotID}: plainMember(testBotID)}}, chatErr: errors.New("unavailable")}
		assertControlAssignment(t, controlAssignmentStore(t, caller), false)
	})
}

func assertControlAssignment(t *testing.T, store *settings.Store, allowed bool) {
	t.Helper()
	group, _ := store.Settings(controlRound2Group)
	next := group.Overrides()
	next.ControlChatID = new(controlRound2Chat)
	_, err := store.Update(group.ID(), group.Revision(), next, controlRound2Actor)
	if (err == nil) != allowed {
		t.Fatalf("assignment error=%v allowed=%t", err, allowed)
	}
	group, _ = store.Settings(controlRound2Group)
	wantID, wantRevision := int64(0), uint64(0)
	if allowed {
		wantID, wantRevision = controlRound2Chat, 1
	}
	if group.ControlChatID().Value != wantID || group.Revision() != wantRevision {
		t.Fatalf("assignment value=%d revision=%d, want %d/%d", group.ControlChatID().Value, group.Revision(), wantID, wantRevision)
	}
	if !allowed && !errors.Is(err, settings.ErrControlChatInvalid) {
		t.Fatalf("error=%v", err)
	}
}
