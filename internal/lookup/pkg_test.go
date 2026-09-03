package lookup

import (
	"strings"
	"testing"

	"github.com/Zakkaus/vestibule/internal/i18n"
)

func TestPkgKeywordLegend(t *testing.T) {
	renderers := []struct {
		name string
		fn   func(i18n.Lang) string
	}{
		{
			name: "plain",
			fn: func(l i18n.Lang) string {
				return renderPkg(l, "vim", []string{"app-editors/vim"}, map[string][2]string{"app-editors/vim": {"", "9.1"}}, nil, pkgLookupAvailability{official: true})
			},
		},
		{
			name: "rich",
			fn: func(l i18n.Lang) string {
				return renderPkgRich(l, "vim", []string{"app-editors/vim"}, map[string][2]string{"app-editors/vim": {"", "9.1"}}, nil, pkgLookupAvailability{official: true})
			},
		},
	}
	for _, l := range i18n.Languages() {
		for _, tt := range renderers {
			t.Run(l.String()+"/"+tt.name, func(t *testing.T) {
				got := tt.fn(l)
				legend := i18n.Messages.LookupPackages.Pkg.KeywordLegend.For(l)
				if !strings.Contains(got, legend) {
					t.Errorf("rendered package result %q does not contain legend %q", got, legend)
				}
				if !strings.Contains(got, "~9.1") {
					t.Errorf("rendered package result does not mark the no-amd64-stable latest version: %q", got)
				}
			})
		}
	}
}

func TestPastedPackageURLsBecomeAtomsWithoutQueryOrFragment(t *testing.T) {
	for _, tc := range []struct {
		name  string
		query string
		want  string
	}{
		{
			name:  "Gentoo package page query",
			query: "https://packages.gentoo.org/packages/www-client/firefox?full=1",
			want:  "www-client/firefox",
		},
		{
			name:  "Gentoo JSON fragment",
			query: "https://packages.gentoo.org/packages/www-client/firefox.json#use-flags",
			want:  "www-client/firefox",
		},
		{
			name:  "GitHub tree query",
			query: "https://github.com/gentoo/gentoo/tree/master/app-editors/vim?plain=1",
			want:  "app-editors/vim",
		},
		{
			name:  "GitHub blob fragment",
			query: "https://github.com/gentoo/gentoo/blob/master/app-editors/vim/vim-9.1.ebuild#L1",
			want:  "app-editors/vim",
		},
		{
			name:  "atom stays unchanged",
			query: "app-editors/vim",
			want:  "app-editors/vim",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeQuery(tc.query); got != tc.want {
				t.Errorf("pasted package URL %q became %q, want atom %q without query or fragment", tc.query, got, tc.want)
			}
		})
	}
}
