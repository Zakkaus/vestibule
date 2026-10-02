package telegram

import (
	"context"
	"errors"
	"testing"

	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/mymmrac/telego"
)

func TestControlChatAssignmentChecksLiveBotMembership(t *testing.T) {
	const controlID int64 = -1009000000651
	for _, tc := range []struct {
		name      string
		member    telego.ChatMember
		lookupErr error
		allowed   bool
	}{
		{"member", plainMember(testBotID), nil, true},
		{"admin", adminMember(testBotID), nil, true},
		{"restricted member", &telego.ChatMemberRestricted{Status: telego.MemberStatusRestricted, IsMember: true}, nil, true},
		{"restricted non-member", &telego.ChatMemberRestricted{Status: telego.MemberStatusRestricted}, nil, false},
		{"left", &telego.ChatMemberLeft{Status: telego.MemberStatusLeft}, nil, false},
		{"banned", &telego.ChatMemberBanned{Status: telego.MemberStatusBanned}, nil, false},
		{"unavailable", nil, errors.New("unavailable"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &settings.Config{GroupIDs: []int64{-1009000000652}}
			baseline, err := settings.LoadBaseline("", cfg)
			if err != nil {
				t.Fatal(err)
			}
			store, err := settings.NewStore("", baseline, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			caller := &controlChatCaller{registrationCaller: &registrationCaller{
				members:      map[[2]int64]telego.ChatMember{{controlID, testBotID}: tc.member, {controlID, 7}: adminMember(7)},
				memberErrors: map[[2]int64]error{{controlID, testBotID}: tc.lookupErr}},
				chatType: telego.ChatTypeSupergroup}
			newRegistrationService(context.Background(), newRegistrationBot(t, caller), store, cfg, "test_bot", testBotID, nil, nil, nil)
			group, _ := store.Settings(-1009000000652)
			next := group.Overrides()
			id := controlID
			next.ControlChatID = &id
			_, err = store.Update(group.ID(), group.Revision(), next, 7)
			if (err == nil) != tc.allowed {
				t.Fatalf("assignment error=%v, allowed=%t", err, tc.allowed)
			}
			if !tc.allowed && !errors.Is(err, settings.ErrControlChatInvalid) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}
