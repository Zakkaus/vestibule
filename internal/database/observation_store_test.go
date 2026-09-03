package database

import (
	"context"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/verification"
)

func TestObservationStorePersistsSanitizedActionsAndCascadesSubjects(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, testSQLiteConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	now := time.Unix(1_800_000_123, 0)
	store, err := NewObservationStore(ctx, db, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	actions := []verification.ObservedAction{
		{Operation: verification.ObservedSend, Flag: true},
		{
			Operation: verification.ObservedBan,
			ChatID:    -1009000000621,
			UserID:    9000000622,
			Seconds:   3600,
			Flag:      true,
		},
	}
	for _, action := range actions {
		if err = store.RecordObservedAction(ctx, action); err != nil {
			t.Fatal(err)
		}
	}

	stored := observedActionsByOperation(t, ctx, store)
	for _, want := range actions {
		got, ok := stored[want.Operation]
		if !ok || got.ObservedAction != want || got.ID == "" || got.ObservedAt != now.Unix() {
			t.Fatalf("stored %s = %+v, want %+v at %d", want.Operation, got, want, now.Unix())
		}
	}
	if _, err = db.Exec(ctx, "DELETE FROM chat WHERE id=$1", actions[1].ChatID); err != nil {
		t.Fatal(err)
	}
	stored = observedActionsByOperation(t, ctx, store)
	if len(stored) != 1 || stored[verification.ObservedSend].Operation != verification.ObservedSend {
		t.Fatalf("observations after group deletion = %+v, want only identifier-free send", stored)
	}
}

func TestObservationStoreSurvivesIndependentSchemaReopen(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, testSQLiteConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	first, err := NewObservationStore(ctx, db, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if err = first.RecordObservedAction(ctx, verification.ObservedAction{Operation: verification.ObservedAckFast}); err != nil {
		t.Fatal(err)
	}
	second, err := NewObservationStore(ctx, db, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := second.LoadObservedActions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 || stored[0].Operation != verification.ObservedAckFast {
		t.Fatalf("reopened observations = %+v, want one ack_fast", stored)
	}
}

func TestObservationStoreRejectsUnclassifiedOrIdentifyingMessageWrites(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, testSQLiteConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store, err := NewObservationStore(ctx, db, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []verification.ObservedAction{
		{Operation: "future_write"},
		{Operation: verification.ObservedSend, ChatID: -1009000000623, UserID: 9000000624},
		{Operation: verification.ObservedApproveJoin},
	} {
		if err = store.RecordObservedAction(ctx, action); err == nil {
			t.Errorf("RecordObservedAction(%+v) succeeded", action)
		}
	}
}

func observedActionsByOperation(
	t *testing.T,
	ctx context.Context,
	store *ObservationStore,
) map[verification.ObservedOperation]StoredObservedAction {
	t.Helper()
	actions, err := store.LoadObservedActions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	indexed := make(map[verification.ObservedOperation]StoredObservedAction, len(actions))
	for _, action := range actions {
		indexed[action.Operation] = action
	}
	return indexed
}

func TestObservationStoreAcceptsEverySuppressedWriteShape(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, testSQLiteConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store, err := NewObservationStore(ctx, db, time.Now)
	if err != nil {
		t.Fatal(err)
	}

	wantActions := everyObservedAction()
	for _, action := range wantActions {
		if err = store.RecordObservedAction(ctx, action); err != nil {
			t.Fatalf("%s observation was rejected before cutover comparison: %v", action.Operation, err)
		}
	}
	stored := observedActionsByOperation(t, ctx, store)
	if len(stored) != len(wantActions) {
		t.Fatalf("stored %d observed write shapes, want all %d", len(stored), len(wantActions))
	}
	for _, want := range wantActions {
		if got := stored[want.Operation].ObservedAction; got != want {
			t.Errorf("stored %s observation = %+v, want %+v", want.Operation, got, want)
		}
	}
}

func everyObservedAction() []verification.ObservedAction {
	const (
		chatID = int64(-1009000000625)
		userID = int64(9000000626)
	)
	return []verification.ObservedAction{
		{Operation: verification.ObservedSend, Flag: true},
		{Operation: verification.ObservedSendHTMLFallback},
		{Operation: verification.ObservedDelete},
		{Operation: verification.ObservedNotify, Seconds: 60},
		{Operation: verification.ObservedAlert},
		{Operation: verification.ObservedAuditLog},
		{Operation: verification.ObservedFailAlert},
		{Operation: verification.ObservedApproveJoin, ChatID: chatID, UserID: userID},
		{Operation: verification.ObservedDeclineJoin, ChatID: chatID, UserID: userID},
		{Operation: verification.ObservedBan, ChatID: chatID, UserID: userID, Seconds: 3600, Flag: true},
		{Operation: verification.ObservedUnban, ChatID: chatID, UserID: userID, Flag: true},
		{Operation: verification.ObservedMute, ChatID: chatID, UserID: userID, Seconds: 180},
		{Operation: verification.ObservedUnmute, ChatID: chatID, UserID: userID},
		{Operation: verification.ObservedAckFast},
		{Operation: verification.ObservedAckResult, Flag: true},
	}
}
