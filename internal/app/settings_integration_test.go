package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/config"
	"github.com/Zakkaus/vestibule/internal/database"
	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/lookup"
	"github.com/Zakkaus/vestibule/internal/store"
	"github.com/Zakkaus/vestibule/internal/telegram"
	"github.com/Zakkaus/vestibule/internal/verification"
)

const testLookupGroup int64 = -1

type testLookupApplication struct {
	cfg      *config.Config
	settings *store.Settings
	verifier *verification.Service
}

func newTestApplication(t *testing.T, ttl *int) *testLookupApplication {
	cfg := &config.Config{GroupIDs: []int64{testLookupGroup},
		Questions:        []config.Question{{Q: "x", Options: []string{"a", "b"}, Answer: 0}},
		LookupTTLSeconds: ttl}
	settings, err := store.NewSettings("", botTestSettingsBaseline(t, cfg))
	if err != nil {
		panic(err)
	}
	verifier := newTestVerifier(settings, nil, cfg, verification.Identity{}, "")
	return &testLookupApplication{cfg: cfg, settings: settings, verifier: verifier}
}

func newTestVerifier(
	settings *store.Settings,
	connector *telegram.Connector,
	cfg *config.Config,
	identity verification.Identity,
	stateDirectory string,
) *verification.Service {
	var gateway verification.Gateway
	if connector != nil {
		gateway = telegram.NewVerificationGateway(connector)
	}
	return verification.New(
		settings, gateway, database.NewVerificationJSONStore(), cfg, &i18n.Messages, nil, identity, stateDirectory,
	)
}

func testLookupService(v *testLookupApplication) *lookup.Service {
	return lookup.New(v.settings, nil, v.cfg, "")
}

func TestLookupAutoDelete(t *testing.T) {
	// default: unset => enabled, 3 minutes
	if ttl, on := testLookupService(newTestApplication(t, nil)).AutoDelete(testLookupGroup); !on || ttl != 3*time.Minute {
		t.Errorf("default = (%v, %v), want (3m, true)", ttl, on)
	}
	// config 0 => disabled
	zero := 0
	disabled := newTestApplication(t, &zero)
	if _, on := testLookupService(disabled).AutoDelete(testLookupGroup); on {
		t.Errorf("lookup_ttl_seconds=0 should disable")
	}
	if err := disabled.verifier.SetAutoDelete(testLookupGroup, 0, true); err != nil {
		t.Fatal(err)
	}
	if ttl, on := testLookupService(disabled).AutoDelete(testLookupGroup); !on || ttl != 3*time.Minute {
		t.Errorf("re-enabled zero baseline = (%v, %v), want (3m, true)", ttl, on)
	}
	// config positive => enabled with that duration
	seconds := 600
	if ttl, on := testLookupService(newTestApplication(t, &seconds)).AutoDelete(testLookupGroup); !on || ttl != 10*time.Minute {
		t.Errorf("lookup_ttl_seconds=600 = (%v, %v), want (10m, true)", ttl, on)
	}
	// runtime: set minutes, then disable — the TTL must persist for a later re-enable
	v := newTestApplication(t, nil)
	if err := v.verifier.SetAutoDelete(testLookupGroup, 5*time.Minute, true); err != nil {
		t.Fatal(err)
	}
	if err := v.verifier.SetAutoDelete(testLookupGroup, 0, false); err != nil {
		t.Fatal(err)
	}
	if ttl, on := testLookupService(v).AutoDelete(testLookupGroup); on || ttl != 5*time.Minute {
		t.Errorf("after off = (%v, %v), want (5m, false)", ttl, on)
	}
}

func botTestSettingsBaseline(t *testing.T, cfg *config.Config) store.SettingsBaseline {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	baseline, err := store.LoadBaseline(path, cfg)
	if err != nil {
		t.Fatal(err)
	}
	return baseline
}
