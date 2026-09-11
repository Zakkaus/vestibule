package telegram

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/Zakkaus/vestibule/internal/console/api"
	"github.com/mymmrac/telego"
	tu "github.com/mymmrac/telego/telegoutil"
)

const chatAdministratorsRequestTimeout = 3 * time.Second

// ChatAdministrators reads the current chat type and administrator privileges from Telegram.
// The API layer owns the result types so this adapter never exposes telego values to callers.
func (c *Connector) ChatAdministrators(ctx context.Context, chatID int64) (api.ChatAdministrators, error) {
	lookupCtx, cancel := context.WithTimeout(ctx, chatAdministratorsRequestTimeout)
	defer cancel()

	chat, err := c.bot.GetChat(lookupCtx, &telego.GetChatParams{ChatID: tu.ID(chatID)})
	if err != nil {
		return api.ChatAdministrators{}, fmt.Errorf("get chat: %w", err)
	}
	if chat == nil || lookupCtx.Err() != nil {
		return api.ChatAdministrators{}, errors.New("get chat returned no result")
	}
	if !supportedChatType(chat.Type) {
		return api.ChatAdministrators{}, fmt.Errorf("unsupported chat type %q", chat.Type)
	}

	members, err := c.bot.GetChatAdministrators(lookupCtx, &telego.GetChatAdministratorsParams{
		ChatID: tu.ID(chatID),
	})
	if err != nil {
		return api.ChatAdministrators{}, fmt.Errorf("get chat administrators: %w", err)
	}
	if lookupCtx.Err() != nil {
		return api.ChatAdministrators{}, lookupCtx.Err()
	}

	result := api.ChatAdministrators{
		Type:    chat.Type,
		IsForum: chat.IsForum,
		Members: make([]api.ChatAdministrator, 0, len(members)),
	}
	for _, member := range members {
		converted, err := convertChatAdministrator(member, chat.Type, chat.IsForum)
		if err != nil {
			return api.ChatAdministrators{}, err
		}
		result.Members = append(result.Members, converted)
	}
	return result, nil
}

func supportedChatType(chatType string) bool {
	return chatType == telego.ChatTypeGroup || chatType == telego.ChatTypeSupergroup || chatType == telego.ChatTypeChannel
}

func convertChatAdministrator(
	member telego.ChatMember,
	chatType string,
	isForum bool,
) (api.ChatAdministrator, error) {
	if member == nil {
		return api.ChatAdministrator{}, errors.New("get chat administrators returned a nil member")
	}
	status := member.MemberStatus()
	if status != telego.MemberStatusCreator && status != telego.MemberStatusAdministrator {
		return api.ChatAdministrator{}, fmt.Errorf("unexpected administrator status %q", status)
	}
	user := member.MemberUser()
	permissions, err := chatAdministratorPermissions(member, chatType, isForum)
	if err != nil {
		return api.ChatAdministrator{}, err
	}
	return api.ChatAdministrator{
		User: api.ChatUser{
			ID:        strconv.FormatInt(user.ID, 10),
			FirstName: user.FirstName,
			LastName:  user.LastName,
			Username:  user.Username,
		},
		Status:      status,
		Permissions: permissions,
	}, nil
}

func chatAdministratorPermissions(
	member telego.ChatMember,
	chatType string,
	isForum bool,
) (api.ChatAdministratorPermissions, error) {
	permissions := api.ChatAdministratorPermissions{}
	if member.MemberStatus() == telego.MemberStatusCreator {
		granted := true
		permissions.CanManageChat = &granted
		permissions.CanDeleteMessages = &granted
		permissions.CanManageVideoChats = &granted
		permissions.CanPromoteMembers = &granted
		permissions.CanChangeInfo = &granted
		permissions.CanInviteUsers = &granted
		permissions.CanPostStories = &granted
		permissions.CanEditStories = &granted
		permissions.CanDeleteStories = &granted
		if chatType != telego.ChatTypeChannel {
			permissions.CanRestrictMembers = &granted
			permissions.CanPinMessages = &granted
		}
		if chatType == telego.ChatTypeChannel {
			permissions.CanPostMessages = &granted
			permissions.CanEditMessages = &granted
		}
		if chatType == telego.ChatTypeSupergroup && isForum {
			permissions.CanManageTopics = &granted
		}
		return permissions, nil
	}

	admin, ok := member.(*telego.ChatMemberAdministrator)
	if !ok {
		return api.ChatAdministratorPermissions{}, errors.New("administrator member has an unexpected type")
	}
	permissions.CanManageChat = &admin.CanManageChat
	permissions.CanDeleteMessages = &admin.CanDeleteMessages
	permissions.CanManageVideoChats = &admin.CanManageVideoChats
	permissions.CanPromoteMembers = &admin.CanPromoteMembers
	permissions.CanChangeInfo = &admin.CanChangeInfo
	permissions.CanInviteUsers = &admin.CanInviteUsers
	permissions.CanPostStories = &admin.CanPostStories
	permissions.CanEditStories = &admin.CanEditStories
	permissions.CanDeleteStories = &admin.CanDeleteStories
	if chatType != telego.ChatTypeChannel {
		permissions.CanRestrictMembers = &admin.CanRestrictMembers
		permissions.CanPinMessages = &admin.CanPinMessages
	}
	if chatType == telego.ChatTypeChannel {
		permissions.CanPostMessages = &admin.CanPostMessages
		permissions.CanEditMessages = &admin.CanEditMessages
	}
	if chatType == telego.ChatTypeSupergroup && isForum {
		permissions.CanManageTopics = &admin.CanManageTopics
	}
	return permissions, nil
}
