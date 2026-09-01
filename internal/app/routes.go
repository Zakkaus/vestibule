package app

import (
	"github.com/Zakkaus/vestibule/internal/lookup"
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
	lookups *lookup.Service,
	console th.Handler,
) telegram.HandlerSet {
	return telegram.HandlerSet{
		Verification: telegram.NewVerificationHandlers(verification, verificationGateway),
		Panel: telegram.PanelHandlers{
			SettingsCallback: administration.OnSettingsCallback,
			ChatShared:       administration.OnPanelChatShared,
			Input:            administration.OnPanelInput,
			Ping:             administration.OnPing,
			Start:            administration.OnStart,
			Settings:         administration.OnSettings,
			Stop:             administration.OnStop,
			Stats:            administration.OnStats,
			Rich:             administration.OnRich,
			Spoiler:          administration.OnSpoiler,
			VerifyMode:       administration.OnVMode,
			AutoDelete:       administration.OnAutoDel,
			BanTime:          administration.OnBanTime,
			Help:             administration.OnHelp,
			ChatSharedDM:     administration.PanelChatSharedDM,
			InputDM:          administration.PanelInputDM,
			SettingsPrefix:   panel.SettingsCallbackPrefix,
		},
		Moderation: telegram.NewModerationHandlers(moderation),
		Lookup: telegram.LookupHandlers{
			Package:  lookups.OnPkg,
			Use:      lookups.OnUse,
			Bug:      lookups.OnBug,
			News:     lookups.OnNews,
			Wiki:     lookups.OnWiki,
			Forum:    lookups.OnBbs,
			Distros:  lookups.OnPkgs,
			Arm:      lookups.OnArm,
			ArmPkgs:  lookups.OnArmpkgs,
			Kernel:   lookups.OnKernel,
			Man:      lookups.OnMan,
			CVE:      lookups.OnCVE,
			Repology: lookups.OnRepology,
		},
		Console: console,
	}
}
