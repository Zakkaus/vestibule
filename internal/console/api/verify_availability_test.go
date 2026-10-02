package api

import (
	"testing"

	"github.com/Zakkaus/vestibule/internal/settings"
)

func TestWebModeAvailabilityDoesNotRequireRegisteredGroup(t *testing.T) {
	f := newAPIWebFixture(t, settings.ModePoW, "en")
	if reason := f.service.WebModeUnavailable(0, settings.ModePoW); reason != "" {
		t.Fatalf("instance PoW capability=%s", reason)
	}
	f.settings.SetWebCapabilities(settings.WebCapabilities{})
	if reason := f.service.WebModeUnavailable(0, settings.ModePoW); reason != "https_required" {
		t.Fatalf("instance capability loss=%s", reason)
	}
	view, _ := f.settings.Settings(apiWebChatID)
	if view.VerifyMode().Value != settings.ModePoW {
		t.Fatal("capability lookup rewrote the stored mode")
	}
}
