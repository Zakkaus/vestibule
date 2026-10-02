package database

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/verification"
)

const importedHoldGroup int64 = -1009000000711

func importHoldRecords(t *testing.T, db *Database, policy PendingDisposition, records ...verification.PendingRecord) error {
	t.Helper()
	directory := copyLegacyFixtures(t)
	data, err := json.Marshal(records)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(directory, "pending.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = ImportLegacyState(context.Background(), db, ImportOptions{
		StateDirectory: directory, BackupDirectory: filepath.Join(t.TempDir(), "backup"), Pending: policy,
	})
	return err
}

func TestCarryCancelsDroppedHoldsOnlyForRestoredMembers(t *testing.T) {
	for _, failCarry := range []bool{false, true} {
		t.Run(map[bool]string{false: "carry", true: "rollback"}[failCarry], func(t *testing.T) {
			ctx := context.Background()
			db, err := Open(ctx, testDatabaseConfig(t))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			restored := verification.PendingRecord{GroupID: importedHoldGroup, UserID: 701, Nonce: "restored", Held: true, HoldUntil: 2000000000}
			other := restored
			other.GroupID-- // Same member in another group must still be released.
			if err = importHoldRecords(t, db, PendingDrop, restored, other); err != nil {
				t.Fatal(err)
			}
			if failCarry {
				if _, err = execTestTrigger(t, ctx, db, `CREATE TRIGGER reject_carry BEFORE DELETE ON warning_counter
					BEGIN SELECT RAISE(ABORT, 'carry rolled back'); END`); err != nil {
					t.Fatal(err)
				}
			}
			err = importHoldRecords(t, db, PendingCarry, restored)
			if (err != nil) != failCarry {
				t.Fatalf("carry error = %v, failure expected = %t", err, failCarry)
			}
			assertCarriedReleaseState(t, db, failCarry, restored, other)
		})
	}
}

func TestDroppedHoldIdentityDistinguishesNewHoldsAndDeduplicatesReplay(t *testing.T) {
	for _, completed := range []bool{false, true} {
		for _, changeNonce := range []bool{false, true} {
			name := map[bool]string{false: "pending", true: "done"}[completed] + map[bool]string{false: "/new-until", true: "/new-nonce"}[changeNonce]
			t.Run(name, func(t *testing.T) {
				db, err := Open(context.Background(), testDatabaseConfig(t))
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = db.Close() })
				old := verification.PendingRecord{GroupID: importedHoldGroup, UserID: 701, Nonce: "old", Held: true, HoldUntil: 2000000000}
				if err = importHoldRecords(t, db, PendingDrop, old); err != nil {
					t.Fatal(err)
				}
				store := NewVerificationStore(db)
				now := time.Now().Add(time.Second).Unix()
				if completed {
					completeImportedHold(t, store, now)
				}
				if err = importHoldRecords(t, db, PendingDrop, old); err != nil {
					t.Fatal(err)
				}
				requireImportedReleaseCount(t, db, 1)
				next := old
				if changeNonce {
					next.Nonce = "new"
				} else {
					next.HoldUntil += 60
				}
				if err = importHoldRecords(t, db, PendingDrop, next); err != nil {
					t.Fatal(err)
				}
				requireImportedReleaseCount(t, db, 2)
				now = time.Now().Add(time.Second).Unix()
				actions, err := store.ClaimImportedUnrestrict("new-worker", now, now+30, 10)
				want := 2
				if completed {
					want = 1
				}
				if err != nil || len(actions) != want {
					t.Fatalf("new-hold releases = %+v, %v; want %d", actions, err, want)
				}
				assertNewHoldPayload(t, actions, next.HoldUntil)
			})
		}
	}
}

func assertNewHoldPayload(t *testing.T, actions []verification.PendingAction, until int64) {
	t.Helper()
	for _, action := range actions {
		var hold ImportedHold
		if err := json.Unmarshal([]byte(action.Payload), &hold); err != nil {
			t.Fatal(err)
		}
		if hold.HoldUntil == until && hold.ChatID == importedHoldGroup && hold.UserID == 701 {
			return
		}
	}
	t.Fatalf("new hold timestamp %d is missing from releases: %+v", until, actions)
}

func assertCarriedReleaseState(t *testing.T, db *Database, failed bool, restored, other verification.PendingRecord) {
	t.Helper()
	store := NewVerificationStore(db)
	pending, err := store.LoadPending("")
	if err != nil {
		t.Fatal(err)
	}
	wantPending, wantReleases := 1, 1
	if failed {
		wantPending, wantReleases = 0, 2
	}
	if len(pending) != wantPending || (!failed && pending[0].Nonce != restored.Nonce) {
		t.Fatalf("carried challenges = %+v", pending)
	}
	now := time.Now().Add(time.Second).Unix()
	actions, err := store.ClaimImportedUnrestrict("first-start", now, now+30, 10)
	if err != nil || len(actions) != wantReleases {
		t.Fatalf("first-start releases = %+v, %v; want %d", actions, err, wantReleases)
	}
	if failed {
		return
	}
	var hold ImportedHold
	if err = json.Unmarshal([]byte(actions[0].Payload), &hold); err != nil {
		t.Fatal(err)
	}
	if hold.ChatID != other.GroupID || hold.UserID != other.UserID {
		t.Fatalf("carry left a release for the restored member: %+v", hold)
	}
	var anchors int
	if err = db.QueryRow(context.Background(), "SELECT count(*) FROM challenge WHERE chat_id=$1 AND reason='import-drop'", restored.GroupID).Scan(&anchors); err != nil {
		t.Fatal(err)
	}
	if anchors != 0 {
		t.Fatalf("carry retained %d outstanding release anchors", anchors)
	}
}

func completeImportedHold(t *testing.T, store *VerificationStore, now int64) {
	t.Helper()
	actions, err := store.ClaimImportedUnrestrict("old-worker", now, now+30, 10)
	if err != nil || len(actions) != 1 {
		t.Fatalf("old release = %+v, %v", actions, err)
	}
	changed, err := store.CompleteAction("", actions[0].ID, "old-worker", now, nil)
	if err != nil || !changed {
		t.Fatalf("complete old release = %t, %v", changed, err)
	}
}

func requireImportedReleaseCount(t *testing.T, db *Database, want int) {
	t.Helper()
	var count int
	if err := db.QueryRow(context.Background(), "SELECT count(*) FROM pending_action WHERE kind='unrestrict'").Scan(&count); err != nil || count != want {
		t.Fatalf("imported hold releases = %d, want %d: %v", count, want, err)
	}
}
