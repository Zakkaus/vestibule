package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Upgrading the settings file copies each recorded field forward. Thirty-six rules do that,
// and probing them one at a time found two nobody was holding: the owner claim's nonce and
// its expiry. Dropping either silently voids a claim that is out but not yet used -- the
// operator follows the link they were sent and it is no longer the one the instance expects,
// with nothing said anywhere. owner_id beside them is covered, so an instance already claimed
// survives; the loss falls on an instance in the middle of being claimed.
func TestUpgradeKeepsAnOutstandingOwnerClaim(t *testing.T) {
	const nonce = "claim-nonce-that-must-survive"
	const expiresAt int64 = 4_070_000_000
	path := filepath.Join(t.TempDir(), "settings.json")
	source, err := json.Marshal(map[string]any{
		"version":                1,
		"registration_revision":  7,
		"owner_id":               0,
		"owner_claim_nonce":      nonce,
		"owner_claim_expires_at": expiresAt,
		"groups":                 map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, source, 0o600); err != nil {
		t.Fatal(err)
	}

	upgraded, err := upgradeSettingsFile(path, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if upgraded.OwnerClaimNonce != nonce {
		t.Errorf("owner_claim_nonce after upgrade = %q, want %q; the link already sent to the "+
			"operator stops being the one this instance accepts",
			upgraded.OwnerClaimNonce, nonce)
	}
	if upgraded.OwnerClaimExpiresAt != expiresAt {
		t.Errorf("owner_claim_expires_at after upgrade = %d, want %d; a claim with no expiry "+
			"reads as one that already lapsed", upgraded.OwnerClaimExpiresAt, expiresAt)
	}
	if upgraded.RegistrationRevision != 7 {
		t.Errorf("registration_revision after upgrade = %d, want 7",
			upgraded.RegistrationRevision)
	}
}
