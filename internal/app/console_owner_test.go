package app

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/settings"
)

// One comparison decides who may hold an operator session, and a console link is that
// session. Written inline in the auth wiring it was unreachable: replacing it with "any
// non-zero Telegram id is the owner" left every test in the repository passing.
func TestOperatorIsOwner(t *testing.T) {
	const owner, stranger = int64(4242), int64(99)
	cfg := &settings.Config{}
	store, err := settings.NewStore(filepath.Join(t.TempDir(), "settings.json"),
		botTestSettingsBaseline(t, cfg), nil)
	if err != nil {
		t.Fatal(err)
	}
	isOwner := operatorIsOwner(store)

	// Before the claim there is no owner, and nobody may hold a session -- least of all the
	// account whose id would be zero if a caller ever supplied one.
	for _, id := range []int64{owner, stranger, 0} {
		if isOwner(id) {
			t.Fatalf("an unclaimed instance treated %d as its owner", id)
		}
	}

	now := time.Now()
	current := store.Registrations()
	next := current
	next.OwnerClaimNonce = "claim-nonce"
	next.OwnerClaimExpiresAt = now.Add(time.Hour).Unix()
	if _, err := store.CommitRegistrations(current.Revision, next); err != nil {
		t.Fatal(err)
	}
	if err := store.ClaimOwner(owner, "claim-nonce", now); err != nil {
		t.Fatal(err)
	}

	if !isOwner(owner) {
		t.Error("the account that claimed the instance is not recognised as its owner")
	}
	if isOwner(stranger) {
		t.Error("a stranger is recognised as the owner; a console link is an operator session")
	}
	if isOwner(0) {
		t.Error("zero is not a Telegram account and must never match")
	}
}
