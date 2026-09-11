package telegram

import (
	"context"
	"errors"
	"testing"

	"github.com/Zakkaus/vestibule/internal/console/api"
	"github.com/mymmrac/telego"
)

const chatAdministratorsTestChatID int64 = -1009000002418

func TestChatAdministratorsMapsOwnerAndAdministratorRights(t *testing.T) {
	owner := &telego.ChatMemberOwner{
		Status: telego.MemberStatusCreator,
		User:   telego.User{ID: 9000002419, FirstName: "Mira", LastName: "Owner", Username: "mira_owner"},
	}
	administrator := &telego.ChatMemberAdministrator{
		Status:              telego.MemberStatusAdministrator,
		User:                telego.User{ID: 9000002420, FirstName: "Ada"},
		CanManageChat:       true,
		CanDeleteMessages:   true,
		CanManageVideoChats: false,
		CanRestrictMembers:  false,
		CanPromoteMembers:   true,
		CanChangeInfo:       true,
		CanInviteUsers:      false,
		CanPostStories:      true,
		CanEditStories:      false,
		CanDeleteStories:    true,
		CanManageTopics:     true,
	}
	caller := &scriptedCaller{responses: map[string][]scriptedResult{
		"getChat": {{value: &telego.ChatFullInfo{
			ID: chatAdministratorsTestChatID, Type: telego.ChatTypeSupergroup, IsForum: true,
		}}},
		"getChatAdministrators": {{value: []telego.ChatMember{owner, administrator}}},
	}}
	client := newTestClient(t, caller)

	got, err := client.ChatAdministrators(context.Background(), chatAdministratorsTestChatID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Type != telego.ChatTypeSupergroup || !got.IsForum || len(got.Members) != 2 {
		t.Fatalf("chat administrators metadata=%+v, want forum supergroup with two members", got)
	}
	assertMappedSupergroupAdministratorRights(t, got.Members)
	if len(caller.methodCalls("getChat")) != 1 || len(caller.methodCalls("getChatAdministrators")) != 1 {
		t.Fatalf("Telegram calls getChat=%d getChatAdministrators=%d, want one each",
			len(caller.methodCalls("getChat")), len(caller.methodCalls("getChatAdministrators")))
	}
}

func assertMappedSupergroupAdministratorRights(t *testing.T, members []api.ChatAdministrator) {
	t.Helper()
	if admin := members[0].Permissions.CanRestrictMembers; members[0].Status != telego.MemberStatusCreator ||
		members[0].User.ID != "9000002419" || admin == nil || !*admin ||
		members[0].Permissions.CanPostMessages != nil {
		t.Fatalf("owner=%+v, want creator with applicable rights and null channel rights", members[0])
	}
	admin := members[1]
	restrict := admin.Permissions.CanRestrictMembers
	topics := admin.Permissions.CanManageTopics
	if admin.Status != telego.MemberStatusAdministrator || admin.User.ID != "9000002420" ||
		restrict == nil || *restrict || topics == nil || !*topics ||
		admin.Permissions.CanPostMessages != nil || admin.Permissions.CanEditMessages != nil {
		t.Fatalf("administrator=%+v, want mapped supergroup rights and null channel rights", admin)
	}
}

func TestChatAdministratorsUsesChannelSpecificPermissions(t *testing.T) {
	administrator := &telego.ChatMemberAdministrator{
		Status:             telego.MemberStatusAdministrator,
		User:               telego.User{ID: 9000002421, FirstName: "Channel Admin"},
		CanManageChat:      true,
		CanPostMessages:    true,
		CanEditMessages:    false,
		CanRestrictMembers: true,
		CanPinMessages:     true,
		CanManageTopics:    true,
	}
	caller := &scriptedCaller{responses: map[string][]scriptedResult{
		"getChat":               {{value: &telego.ChatFullInfo{Type: telego.ChatTypeChannel}}},
		"getChatAdministrators": {{value: []telego.ChatMember{administrator}}},
	}}
	client := newTestClient(t, caller)

	got, err := client.ChatAdministrators(context.Background(), chatAdministratorsTestChatID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Members) != 1 {
		t.Fatalf("channel administrators=%d, want one", len(got.Members))
	}
	member := got.Members[0]
	post := member.Permissions.CanPostMessages
	edit := member.Permissions.CanEditMessages
	if post == nil || !*post || edit == nil || *edit ||
		member.Permissions.CanRestrictMembers != nil ||
		member.Permissions.CanPinMessages != nil ||
		member.Permissions.CanManageTopics != nil {
		t.Fatalf("channel permissions=%+v, want post/edit values and group/forum rights null", member.Permissions)
	}
}

func TestChatAdministratorsReturnsGetChatFailureWithoutListingAdministrators(t *testing.T) {
	caller := &scriptedCaller{responses: map[string][]scriptedResult{
		"getChat": {{err: errors.New("getChat unavailable")}},
	}}
	client := newTestClient(t, caller)

	if _, err := client.ChatAdministrators(context.Background(), chatAdministratorsTestChatID); err == nil {
		t.Fatal("GetChat failure returned nil error")
	}
	if calls := len(caller.methodCalls("getChatAdministrators")); calls != 0 {
		t.Fatalf("getChatAdministrators calls=%d after GetChat failure, want 0", calls)
	}
}

func TestChatAdministratorsReturnsListingFailure(t *testing.T) {
	caller := &scriptedCaller{responses: map[string][]scriptedResult{
		"getChat":               {{value: &telego.ChatFullInfo{Type: telego.ChatTypeGroup}}},
		"getChatAdministrators": {{err: errors.New("administrator list unavailable")}},
	}}
	client := newTestClient(t, caller)

	if _, err := client.ChatAdministrators(context.Background(), chatAdministratorsTestChatID); err == nil {
		t.Fatal("GetChatAdministrators failure returned nil error")
	}
	if len(caller.methodCalls("getChat")) != 1 || len(caller.methodCalls("getChatAdministrators")) != 1 {
		t.Fatalf("Telegram calls getChat=%d getChatAdministrators=%d, want one each",
			len(caller.methodCalls("getChat")), len(caller.methodCalls("getChatAdministrators")))
	}
}
