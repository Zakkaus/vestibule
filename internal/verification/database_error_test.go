package verification

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/settings"
	_ "modernc.org/sqlite"
)

type failingFailureSnapshotStore struct {
	testVerificationStore
	db        *sql.DB
	saveCalls int
}

func (s *failingFailureSnapshotStore) LoadFailures(string) ([]FailureRecord, error) {
	return nil, errors.New("temporary database read failure")
}

func (s *failingFailureSnapshotStore) SaveFailures(_ string, snapshot func() []FailureRecord) error {
	s.saveCalls++
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.Exec("DELETE FROM verification_failure"); err != nil {
		return err
	}
	for _, record := range snapshot() {
		if _, err = tx.Exec(
			"INSERT INTO verification_failure (chat_id, user_id, count, last_at) VALUES (?, ?, ?, ?)",
			record.GroupID, record.UserID, record.Count, record.Last,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func TestFailureLoadErrorBlocksDestructiveSnapshotWrite(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err = db.Exec(`CREATE TABLE verification_failure (
		chat_id INTEGER NOT NULL,
		user_id INTEGER NOT NULL,
		count INTEGER NOT NULL,
		last_at INTEGER NOT NULL
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("INSERT INTO verification_failure VALUES (-100, 7, 3, 1234)"); err != nil {
		t.Fatal(err)
	}

	store := &failingFailureSnapshotStore{db: db}
	service := newTestService(&settings.Config{})
	service.stateStore = store
	service.vfailPath = "database"
	service.loadVerifyFails()
	service.saveVerifyFails()

	var count, strikes int
	if err = db.QueryRow("SELECT COUNT(*), COALESCE(MAX(count), 0) FROM verification_failure").Scan(&count, &strikes); err != nil {
		t.Fatal(err)
	}
	t.Logf("history rows=%d max_count=%d snapshot_writes=%d", count, strikes, store.saveCalls)
	if count != 1 || strikes != 3 {
		t.Fatalf("history after failed load and snapshot = rows %d max count %d, want 1 and 3", count, strikes)
	}
	if store.saveCalls != 0 {
		t.Fatalf("snapshot write ran %d time(s) after its authoritative load failed", store.saveCalls)
	}
}

type errorTransitionStore struct {
	testVerificationStore
	calls int
}

func (s *errorTransitionStore) UpdatePending(string, PendingRef, PendingRecord) (bool, error) {
	return true, nil
}
func (s *errorTransitionStore) TransitionChallenge(string, ChallengeTransition) (bool, error) {
	s.calls++
	return false, errors.New("temporary transition failure")
}

func TestOnAnswerReturnsStoreErrorButKeepsLocalPending(t *testing.T) {
	const gid, uid = int64(-100), int64(5)
	state := &errorTransitionStore{}
	service := newTestService(&settings.Config{VerifyMaxFails: 3})
	service.statePath = "database"
	service.stateStore = state
	key := pkey{gid: gid, uid: uid}
	service.pend[key] = &pending{
		nonce: "current", correctIdx: 1, groupMsgID: 42, deadline: time.Now().Add(time.Hour),
		epoch: 4, persistedPath: service.statePath,
	}
	bot := newFakeVerifyBot()
	update := Update{CallbackQuery: &CallbackQuery{
		ID: "answer", From: User{ID: uid}, Data: "v:-100:5:current:1",
	}}

	err := service.OnAnswer(NewHandlerContext(context.Background(), newAPITestBot(t, bot)), update)
	t.Logf("transition calls=%d handler error=%v approvals=%d pending=%t",
		state.calls, err, bot.approves, service.pend[key] != nil)
	if err == nil {
		t.Fatal("database transition error was reported as an already-settled success")
	}
	if state.calls != storeWriteMaxAttempts || bot.approves != 0 {
		t.Fatalf("transition calls/approvals = %d/%d, want %d/0", state.calls, bot.approves, storeWriteMaxAttempts)
	}
	if service.pend[key] == nil {
		t.Fatal("database transition error discarded the local pending")
	}
}

type flakyAgentStore struct {
	testVerificationStore
	failures int
	calls    int
	saved    AgentTally
}

func (s *flakyAgentStore) SaveAgents(_ string, snapshot func() AgentTally) error {
	s.calls++
	tally := snapshot()
	if s.calls <= s.failures {
		return errors.New("temporary agent tally write failure")
	}
	s.saved = tally
	return nil
}

func TestAgentSnapshotRetriesWithoutRepeatingLocalMutation(t *testing.T) {
	store := &flakyAgentStore{failures: storeWriteMaxAttempts - 1}
	service := newTestService(&settings.Config{})
	service.agentPath = "database"
	service.stateStore = store

	model, total := service.recordAgent("AGENT-N model=gpt-5")
	t.Logf("agent writes=%d model=%s local_total=%d stored_total=%d",
		store.calls, model, total, store.saved.Total)
	if store.calls != storeWriteMaxAttempts {
		t.Fatalf("agent snapshot writes = %d, want %d", store.calls, storeWriteMaxAttempts)
	}
	if model != "gpt-5" || total != 1 || service.agents.Total != 1 || store.saved.Total != 1 {
		t.Fatalf("agent tally after retry = model %q result %d local %d stored %d",
			model, total, service.agents.Total, store.saved.Total)
	}
}
