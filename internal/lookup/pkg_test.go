package lookup

import (
	"testing"
)

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
			if got := NormalizeQuery(tc.query); got != tc.want {
				t.Errorf("pasted package URL %q became %q, want atom %q without query or fragment", tc.query, got, tc.want)
			}
		})
	}
}
