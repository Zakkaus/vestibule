package app

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/database"
	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/lookup"
	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/Zakkaus/vestibule/internal/telegram"
	"github.com/Zakkaus/vestibule/internal/verification"
)

const testLookupGroup int64 = -1

type testLookupApplication struct {
	cfg      *settings.Config
	settings *settings.Store
	verifier *verification.Service
}

func newTestApplication(t *testing.T, ttl *int) *testLookupApplication {
	cfg := &settings.Config{GroupIDs: []int64{testLookupGroup},
		Questions:        []settings.Question{{Q: "x", Options: []string{"a", "b"}, Answer: 0}},
		LookupTTLSeconds: ttl}
	settings, err := settings.NewStore("", botTestSettingsBaseline(t, cfg), nil)
	if err != nil {
		panic(err)
	}
	verifier := newTestVerifier(t, settings, nil, cfg, verification.Identity{}, "")
	return &testLookupApplication{cfg: cfg, settings: settings, verifier: verifier}
}

func newTestVerifier(
	t *testing.T,
	settings *settings.Store,
	connector *telegram.Connector,
	cfg *settings.Config,
	identity verification.Identity,
	stateDirectory string,
) *verification.Service {
	t.Helper()
	var gateway verification.Gateway
	if connector != nil {
		gateway = telegram.NewVerificationGateway(connector)
	}
	service, err := verification.New(
		settings, gateway, database.NewVerificationJSONStore(), cfg, &i18n.Messages, nil, identity, stateDirectory,
	)
	if err != nil {
		t.Fatal(err)
	}
	return service
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

func botTestSettingsBaseline(t *testing.T, cfg *settings.Config) settings.SettingsBaseline {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	data := []byte(`{}`)
	if cfg.LookupTTLSeconds != nil {
		data = []byte(fmt.Sprintf(`{"lookup_ttl_seconds":%d}`, *cfg.LookupTTLSeconds))
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	baseline, err := settings.LoadBaseline(path, cfg)
	if err != nil {
		t.Fatal(err)
	}
	return baseline
}
