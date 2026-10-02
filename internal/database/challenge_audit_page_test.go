package database

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/verification"
)

func TestChallengeAuditKeysetPagesAndOffPageLatestDecision(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, testSQLiteConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := NewVerificationStore(db)
	const chatID int64 = -1009000000911
	for i := 1; i <= 6; i++ {
		record := verification.PendingRecord{GroupID: chatID, UserID: int64(i), Nonce: "page", Deadline: 90, Epoch: 1}
		requireAuditTransition(t, store, record, verification.ChallengeBanned, "", 100, 9)
	}
	newer := verification.PendingRecord{GroupID: chatID, UserID: 1, Nonce: "newer", Deadline: 190, Epoch: 2}
	requireAuditTransition(t, store, newer, verification.ChallengeApproved, "", 200, 9)
	var ids []string
	page := verification.AuditPageRequest{Limit: 2}
	for {
		records := requireBoundedAuditPage(t, store, chatID, page)
		visible := records
		if len(visible) > 2 {
			visible = visible[:2]
		}
		for _, record := range visible {
			ids = append(ids, record.ID)
			if record.Record.UserID == 1 && record.Record.Nonce == "page" && record.Latest {
				t.Fatal("old ban is undoable because its newer decision is on another page")
			}
		}
		if len(records) <= 2 {
			break
		}
		last := visible[len(visible)-1]
		page.Cursor = verification.AuditNextCursor(chatID, verification.ConsoleAuditEntry{ID: last.ID, SettledAt: time.Unix(last.SettledAt, 0)})
		if len(ids) == 2 {
			insert := verification.PendingRecord{GroupID: chatID, UserID: 7, Nonce: "inserted", Deadline: 290, Epoch: 1}
			requireAuditTransition(t, store, insert, verification.ChallengeApproved, "", 300, 9)
		}
	}
	want := []string{challengeID(newer.Ref())}
	for i := 6; i >= 1; i-- {
		want = append(want, fmt.Sprintf("%d:%d:page", chatID, i))
	}
	if !reflect.DeepEqual(ids, want) {
		t.Fatalf("keyset history = %v, want %v", ids, want)
	}
	requireOffPageAuditTarget(t, store, chatID, want[len(want)-1])
}

func requireBoundedAuditPage(t *testing.T, store *VerificationStore, chatID int64, page verification.AuditPageRequest) []verification.ChallengeAuditRecord {
	t.Helper()
	records, err := store.LoadChallengeAudit(context.Background(), chatID, page)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) > page.Limit+1 {
		t.Fatalf("unbounded page: %d rows", len(records))
	}
	return records
}

func requireOffPageAuditTarget(t *testing.T, store *VerificationStore, chatID int64, id string) {
	t.Helper()
	ctx := context.Background()
	target, found, err := store.LoadChallengeAuditByID(ctx, chatID, id)
	if err != nil || !found || target.Latest {
		t.Fatalf("off-page undo target=%#v found=%v error=%v", target, found, err)
	}
	_, found, err = store.LoadChallengeAuditByID(ctx, chatID-1, target.ID)
	if err != nil || found {
		t.Fatalf("cross-group target disclosed: found=%v error=%v", found, err)
	}
}

func TestChallengeAuditRejectsInvalidPages(t *testing.T) {
	db, err := Open(context.Background(), testSQLiteConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := NewVerificationStore(db)
	const chatID int64 = -1009000000912
	foreign := verification.AuditNextCursor(chatID-1, verification.ConsoleAuditEntry{ID: "foreign", SettledAt: time.Unix(100, 0)})
	for _, page := range []verification.AuditPageRequest{{Limit: -1}, {Limit: 101}, {Cursor: "broken"}, {Cursor: foreign}} {
		if _, err := store.LoadChallengeAudit(context.Background(), chatID, page); !errors.Is(err, verification.ErrConsoleAuditInvalid) {
			t.Fatalf("page %#v: error=%v", page, err)
		}
	}
}
