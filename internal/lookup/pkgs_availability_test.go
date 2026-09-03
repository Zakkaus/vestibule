package lookup

import (
	"testing"
)

func TestUnrecordedOverlayAvailabilityFollowsCachedPackageMap(t *testing.T) {
	source := overlay{name: "cached-overlay"}
	tests := []struct {
		name string
		pkgs map[string]map[string]string
		want bool
	}{
		{
			name: "cache already holds the overlay map",
			pkgs: map[string]map[string]string{
				"cached-overlay": {},
			},
			want: true,
		},
		{
			name: "cold cache has no overlay map",
			pkgs: map[string]map[string]string{},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc := &pkgCache{pkgs: tt.pkgs, available: map[string]bool{}}
			if got := pc.availability([]overlay{source})[source.name]; got != tt.want {
				t.Errorf("unrecorded overlay availability = %v, want %v; /pkg must distinguish a cached index from one not loaded", got, tt.want)
			}
		})
	}
}
