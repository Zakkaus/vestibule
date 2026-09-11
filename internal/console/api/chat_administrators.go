package api

import "context"

// ChatAdministratorsResolver supplies current Telegram administrator data without exposing Telegram types to the API.
type ChatAdministratorsResolver interface {
	ChatAdministrators(context.Context, int64) (ChatAdministrators, error)
}

// ChatUser is the identity attached to a Telegram chat owner or administrator.
type ChatUser struct {
	ID        string `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name,omitempty"`
	Username  string `json:"username,omitempty"`
}

// ChatAdministratorPermissions uses pointers so Telegram capabilities can be null when they do not apply.
type ChatAdministratorPermissions struct {
	CanManageChat       *bool `json:"can_manage_chat"`
	CanDeleteMessages   *bool `json:"can_delete_messages"`
	CanManageVideoChats *bool `json:"can_manage_video_chats"`
	CanRestrictMembers  *bool `json:"can_restrict_members"`
	CanPromoteMembers   *bool `json:"can_promote_members"`
	CanChangeInfo       *bool `json:"can_change_info"`
	CanInviteUsers      *bool `json:"can_invite_users"`
	CanPostStories      *bool `json:"can_post_stories"`
	CanEditStories      *bool `json:"can_edit_stories"`
	CanDeleteStories    *bool `json:"can_delete_stories"`
	CanPostMessages     *bool `json:"can_post_messages"`
	CanEditMessages     *bool `json:"can_edit_messages"`
	CanPinMessages      *bool `json:"can_pin_messages"`
	CanManageTopics     *bool `json:"can_manage_topics"`
}

// ChatAdministrator is one current Telegram creator or administrator.
type ChatAdministrator struct {
	User        ChatUser                     `json:"user"`
	Status      string                       `json:"status"`
	Permissions ChatAdministratorPermissions `json:"permissions"`
}

// ChatAdministrators is the Telegram administrator response plus chat type metadata.
type ChatAdministrators struct {
	Type    string
	IsForum bool
	Members []ChatAdministrator
}
