package panel

import (
	"context"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/mymmrac/telego"
)

func (v *Panel) authorizeSettingsCallback(ctx context.Context, bot *telego.Bot, query *telego.CallbackQuery) (callbackData, i18n.Lang, bool) {
	if query.Message == nil || query.From.ID <= 0 || query.From.IsBot {
		v.answerCallback(ctx, bot, query.ID, "", false)
		return callbackData{}, 0, false
	}
	data, err := parseCallback(query.Data)
	if err != nil || v.settings == nil || !v.settings.IsGroup(data.group) {
		v.answerCallback(ctx, bot, query.ID, "", false)
		return callbackData{}, 0, false
	}
	language := i18n.FromTelegram(query.From.LanguageCode)
	admin, authErr := v.isGroupRestrictAdmin(ctx, data.group, query.From.ID)
	if authErr != nil {
		v.answerCallback(ctx, bot, query.ID, i18n.Messages.Panel.Settings.Error.AuthorizationCheckFailed.For(language), true)
		return callbackData{}, 0, false
	}
	if !admin {
		v.finishUserSession(ctx, bot, query.From.ID, query.Message, i18n.Messages.Panel.Settings.Error.AuthorizationLost.For(language))
		v.answerCallback(ctx, bot, query.ID, i18n.Messages.Panel.Settings.Error.AuthorizationLost.For(language), true)
		return callbackData{}, 0, false
	}
	return data, language, true
}
