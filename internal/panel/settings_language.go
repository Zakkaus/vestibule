package panel

import (
	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/settings"
)

func languageCallbackValue(value string) (string, bool) {
	switch value {
	case "z":
		return "zh", true
	case "h":
		return "zh-Hant", true
	case "e":
		return "en", true
	case "j":
		return "ja", true
	case "r":
		return "ru", true
	default:
		return "", false
	}
}

func (v *Panel) sourcedLanguage(language i18n.Lang, setting settings.Setting[string]) string {
	value := map[string]string{
		"zh":      i18n.Messages.Panel.Settings.Field.LanguageZH.For(language),
		"zh-Hant": i18n.Messages.Panel.Settings.Field.LanguageZHHant.For(language),
		"en":      i18n.Messages.Panel.Settings.Field.LanguageEN.For(language),
		"ja":      i18n.Messages.Panel.Settings.Field.LanguageJA.For(language),
		"ru":      i18n.Messages.Panel.Settings.Field.LanguageRU.For(language),
	}[setting.Value]
	return i18n.Messages.Panel.Settings.Value.Sourced.Render(language, value, v.sourceText(language, setting.Source))
}
