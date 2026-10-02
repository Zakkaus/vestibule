package lookup

import (
	"testing"

	"github.com/Zakkaus/vestibule/internal/i18n"
)

// The homepage link text used to be the English word, printed as-is to Chinese readers.
func TestHomepageLabelIsLocalized(t *testing.T) {
	want := map[i18n.Lang]string{
		i18n.LangEN: "homepage", i18n.LangZH: "主页", i18n.LangZHHant: "首頁",
		i18n.LangJA: "ホームページ", i18n.LangRU: "домашняя страница",
	}
	for l, expected := range want {
		if got := i18n.Messages.LookupPackages.Use.Homepage.For(l); got != expected {
			t.Errorf("%v homepage label = %q, want %q", l, got, expected)
		}
	}
}
