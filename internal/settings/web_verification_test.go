package settings

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestWebSettingsValidateResultWithoutAdvancingRevision(t *testing.T) {
	for _, tc := range []struct {
		name, url, mode, delivery string
		provider                  bool
		bits                      int
		policy                    string
		reject                    bool
	}{
		{"pow", "https://console.example", ModePoW, DeliveryDM, false, 12, CaptchaFallback, false},
		{"captcha", "https://console.example:8443", ModeCaptcha, DeliveryBoth, true, 22, CaptchaApprove, false},
		{"missing provider", "https://console.example", ModeCaptcha, DeliveryDM, false, 18, CaptchaFallback, true},
		{"group only", "https://console.example", ModePoW, DeliveryGroup, true, 18, CaptchaFallback, true},
		{"http", "http://console.example", ModePoW, DeliveryDM, true, 18, CaptchaFallback, true},
		{"relative", "/console", ModePoW, DeliveryDM, true, 18, CaptchaFallback, true},
		{"localhost", "https://localhost", ModePoW, DeliveryDM, true, 18, CaptchaFallback, true},
		{"loopback range", "https://127.12.3.4:8443", ModePoW, DeliveryDM, true, 18, CaptchaFallback, true},
		{"IPv6 loopback", "https://[::1]", ModePoW, DeliveryDM, true, 18, CaptchaFallback, true},
		{"difficulty low", "https://console.example", ModePoW, DeliveryDM, true, 11, CaptchaFallback, true},
		{"difficulty high", "https://console.example", ModePoW, DeliveryDM, true, 23, CaptchaFallback, true},
		{"policy", "https://console.example", ModePoW, DeliveryDM, true, 18, "allow", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, err := NewStore("", testSettingsBaseline(), nil, nil)
			requireNoError(t, err)
			store.SetWebCapabilities(WebCapabilities{ConsoleURL: tc.url, TurnstileAvailable: tc.provider})
			next := GroupOverrides{VerifyMode: &tc.mode, DeliveryMode: &tc.delivery, PoWBits: &tc.bits, CaptchaUnavailable: &tc.policy}
			_, err = store.Update(testGroupA, 0, next)
			if (err != nil) != tc.reject {
				t.Fatalf("write error=%v, reject=%v", err, tc.reject)
			}
			view := requireSettingsView(t, store, testGroupA)
			if tc.reject {
				requireEqual(t, view.Revision(), uint64(0), "rejected revision")
				return
			}
			next = view.Overrides()
			next.DeliveryMode = ptr(DeliveryGroup)
			if _, err = store.Update(testGroupA, view.Revision(), next); err == nil {
				t.Fatal("delivery-only patch accepted for web mode")
			}
			requireEqual(t, requireSettingsView(t, store, testGroupA).Revision(), view.Revision(), "delivery-only rejection revision")
		})
	}
}

func TestWebSettingsCopyResetAndCapPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	store, err := NewStore(path, testSettingsBaseline(), nil, nil)
	requireNoError(t, err)
	cap := int64(16)
	_, err = store.UpdateOwnerLimits(0, LimitChanges{"pow_bits": &cap})
	requireNoError(t, err)
	_, err = store.Update(testGroupA, 0, GroupOverrides{PoWBits: ptr(17)})
	requireErrorIs(t, err, ErrOwnerLimitsExceeded, "pow cap")
	bits, policy := 12, CaptchaDecline
	_, err = store.Update(testGroupA, 0, GroupOverrides{PoWBits: &bits, CaptchaUnavailable: &policy})
	requireNoError(t, err)
	bits, policy = 22, CaptchaApprove
	view := requireSettingsView(t, store, testGroupA)
	requireEqual(t, view.PoWBits().Value, 12, "detached input")
	reloaded, err := NewStore(path, testSettingsBaseline(), nil, nil)
	requireNoError(t, err)
	view = requireSettingsView(t, reloaded, testGroupA)
	requireEqual(t, view.PoWBits().Value, 12, "persisted bits")
	requireEqual(t, view.CaptchaUnavailable().Value, CaptchaDecline, "persisted policy")
	_, err = reloaded.Update(testGroupA, view.Revision(), GroupOverrides{})
	if !errors.Is(err, ErrOwnerLimitsExceeded) {
		t.Fatalf("reset above lowered cap error=%v", err)
	}
	_, err = reloaded.UpdateOwnerLimits(1, LimitChanges{"pow_bits": nil})
	requireNoError(t, err)
	_, err = reloaded.Update(testGroupA, view.Revision(), GroupOverrides{})
	requireNoError(t, err)
	requireEqual(t, requireSettingsView(t, reloaded, testGroupA).PoWBits().Value, 18, "reset bits")
}

func TestWebDeliveryConfigDriftPreservesOtherGroupOverrides(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	baseline := testSettingsBaseline()
	store, err := NewStore(path, baseline, nil, nil)
	requireNoError(t, err)
	store.SetWebCapabilities(WebCapabilities{ConsoleURL: "https://console.example"})
	_, err = store.Update(testGroupA, 0, GroupOverrides{VerifyMode: ptr(ModePoW)})
	requireNoError(t, err)
	_, err = store.Update(testGroupB, 0, GroupOverrides{BanSeconds: ptr(77)})
	requireNoError(t, err)
	baseline.Groups[0].DeliveryMode.Value = DeliveryGroup
	reloaded, err := NewStore(path, baseline, nil, nil)
	requireNoError(t, err)
	requireEqual(t, requireSettingsView(t, reloaded, testGroupB).BanSeconds().Value, 77, "unrelated persisted override")
	_, err = reloaded.Update(testGroupB, 1, GroupOverrides{BanSeconds: ptr(78)})
	requireNoError(t, err)
	requireEqual(t, requireSettingsView(t, reloaded, testGroupA).VerifyMode().Value, ModePoW, "stored web mode")
}
