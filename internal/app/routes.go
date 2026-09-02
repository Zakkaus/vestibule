package app

import (
	"github.com/Zakkaus/vestibule/internal/moderate"
	"github.com/Zakkaus/vestibule/internal/panel"
	"github.com/Zakkaus/vestibule/internal/telegram"
	"github.com/Zakkaus/vestibule/internal/verification"
	th "github.com/mymmrac/telego/telegohandler"
)

func telegramHandlers(
	verification *verification.Service,
	verificationGateway *telegram.VerificationGateway,
	administration *panel.Panel,
	moderation *moderate.Service,
	commands telegram.CommandModules,
	console th.Handler,
) telegram.HandlerSet {
	return telegram.HandlerSet{
		Verification: telegram.NewVerificationHandlers(verification, verificationGateway),
		Panel: telegram.PanelHandlers{
			SettingsCallback: administration.OnSettingsCallback,
			ChatShared:       administration.OnPanelChatShared,
			Input:            administration.OnPanelInput,
			ChatSharedDM:     administration.PanelChatSharedDM,
			InputDM:          administration.PanelInputDM,
			SettingsPrefix:   panel.SettingsCallbackPrefix,
		},
		Moderation: telegram.NewModerationHandlers(moderation),
		Commands:   commands,
		Console:    console,
	}
}
