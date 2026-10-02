package tgfmt

import (
	"github.com/Zakkaus/vestibule/internal/i18n"
)

func RenderRepologyLookupMiss(l i18n.Lang, name string, available bool) string {
	if !available {
		return i18n.Messages.LookupDistros.Pkgs.RepologyUnavailable.Render(l, name)
	}
	return i18n.Messages.LookupDistros.Pkgs.RepologyNotFound.Render(l, name)
}
