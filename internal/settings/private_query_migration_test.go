package settings

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"testing"
)

func TestLegacyPrivateQuerySettingsAreIgnoredAndLogged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	data := []byte(`{"version":4,"limits":{"private_query_per_min":1},"groups":{"-1009000000001":{"revision":7,"private_query_per_min":0,"warn_limit":5}}}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	old := log.Writer()
	log.SetOutput(&output)
	defer log.SetOutput(old)
	store, err := NewStore(path, testSettingsBaseline(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	group := requireSettingsView(t, store, testGroupA)
	if group.Revision() != 7 || group.WarnLimit().Value != 5 {
		t.Fatalf("lost unrelated settings: %+v", group)
	}
	next := group.Overrides()
	next.Enabled = ptr(false)
	if _, err := store.Update(testGroupA, 7, next); err != nil {
		t.Fatalf("obsolete cap blocks update: %v", err)
	}
	if !bytes.Contains(output.Bytes(), []byte("private_query_per_min")) {
		t.Fatalf("missing migration warning: %s", output.String())
	}
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(saved, []byte("private_query_per_min")) {
		t.Fatal("obsolete field persisted")
	}
	rate := int64(1)
	if _, err := store.UpdateOwnerLimits(0, LimitChanges{"private_query_per_min": &rate}); err == nil {
		t.Fatal("accepted obsolete group cap")
	}
}
