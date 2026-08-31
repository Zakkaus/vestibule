package tgfmt

import "github.com/Zakkaus/vestibule/internal/i18n"

func ModerationBanDurationText(l i18n.Lang, seconds int) string {
	if seconds <= 0 {
		return i18n.Messages.Moderate.Duration.Permanent.For(l)
	}
	switch {
	case seconds%86400 == 0:
		return i18n.Messages.Moderate.Duration.Days.Render(l, seconds/86400)
	case seconds%3600 == 0:
		return i18n.Messages.Moderate.Duration.Hours.Render(l, seconds/3600)
	case seconds%60 == 0:
		return i18n.Messages.Moderate.Duration.Minutes.Render(l, seconds/60)
	default:
		return i18n.Messages.Moderate.Duration.Seconds.Render(l, seconds)
	}
}

func ModerationBanDurationStatus(l i18n.Lang, seconds int) string {
	effect := i18n.Messages.Moderate.Duration.PermanentEffect.For(l)
	if seconds > 0 {
		effect = i18n.Messages.Moderate.Duration.TemporaryEffect.For(l)
	}
	return i18n.Messages.Moderate.Duration.Status.Render(l, ModerationBanDurationText(l, seconds), effect)
}

func ChannelSenderAlert(l i18n.Lang, banned bool, title string, senderID, groupID int64) string {
	if banned {
		return i18n.Messages.Moderate.Antispam.SenderBannedAlert.Render(l, title, senderID, groupID, senderID)
	}
	return i18n.Messages.Moderate.Antispam.SenderBanFailedAlert.Render(l, title, senderID, groupID)
}
