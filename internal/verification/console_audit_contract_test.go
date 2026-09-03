package verification

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/settings"
)

const (
	consoleAuditContractGroupID int64 = -100900000001
	consoleAuditContractActorID int64 = 901
)

type consoleAuditContractStore struct {
	testVerificationStore
	records   []ChallengeAuditRecord
	allowUndo bool
}

func (s *consoleAuditContractStore) LoadChallengeAudit(context.Context, int64) ([]ChallengeAuditRecord, error) {
	return append([]ChallengeAuditRecord(nil), s.records...), nil
}

func (s *consoleAuditContractStore) EnqueueChallengeUndo(
	_ context.Context,
	expected ChallengeAuditRecord,
	_ ActionIntent,
) (bool, error) {
	if !s.allowUndo {
		return false, nil
	}
	for i := range s.records {
		if s.records[i].ID != expected.ID || s.records[i].UndoAction != ChallengeActionNone {
			continue
		}
		s.records[i].UndoAction = ChallengeActionPending
		return true, nil
	}
	return false, nil
}

func (s *consoleAuditContractStore) CompleteAction(
	_ string,
	id, _ string,
	_ int64,
	_ []ActionIntent,
) (bool, error) {
	for i := range s.records {
		if id != "undo:"+s.records[i].ID+":ban" {
			continue
		}
		s.records[i].UndoAction = ChallengeActionDone
		return true, nil
	}
	return false, nil
}

func TestUndoConsoleAuditConflictsWhenStoreRejectsUndo(t *testing.T) {
	record := consoleAuditContractRecord(
		consoleAuditContractGroupID, 101, "-100900000001:101:ban", ChallengeBanned, 100,
	)
	undo := ConsoleAuditUndo{ID: record.ID, GroupID: record.Record.GroupID, ActorID: consoleAuditContractActorID}

	t.Run("compare-and-swap conflict does not unban", func(t *testing.T) {
		service, gateway := newConsoleAuditContractService([]ChallengeAuditRecord{record}, false)
		entries, err := service.ConsoleAudit(context.Background(), undo.GroupID, undo.ActorID)
		if err != nil || len(entries) != 1 || entries[0].UndoState != ConsoleUndoAvailable {
			t.Fatalf("undoable ban audit entries=%#v error=%v", entries, err)
		}

		_, err = service.UndoConsoleAudit(context.Background(), undo)
		if !errors.Is(err, ErrConsoleAuditConflict) || gateway.unbans != 0 {
			t.Fatalf("compare-and-swap conflict error=%v unbans=%d", err, gateway.unbans)
		}
	})

	t.Run("compare-and-swap success unbans", func(t *testing.T) {
		service, gateway := newConsoleAuditContractService([]ChallengeAuditRecord{record}, true)

		entry, err := service.UndoConsoleAudit(context.Background(), undo)
		if err != nil || entry.UndoState != ConsoleUndoCompleted || gateway.unbans != 1 ||
			len(gateway.unbanned) != 1 || gateway.unbanned[0] != [2]int64{undo.GroupID, record.Record.UserID} {
			t.Fatalf("successful undo entry=%#v error=%v unbans=%#v", entry, err, gateway.unbanned)
		}
	})
}

func TestConsoleAuditRejectsCrossGroupRecords(t *testing.T) {
	requestedGroupID := consoleAuditContractGroupID - 1
	crossGroupRecord := consoleAuditContractRecord(
		requestedGroupID-1, 201, "-100900000003:201:cross-group", ChallengeBanned, 200,
	)

	t.Run("cross-group audit disclosure", func(t *testing.T) {
		service, _ := newConsoleAuditContractService([]ChallengeAuditRecord{crossGroupRecord}, false)
		entries, err := service.ConsoleAudit(context.Background(), requestedGroupID, consoleAuditContractActorID)
		if !errors.Is(err, ErrConsoleAuditUnavailable) || entries != nil {
			t.Fatalf("cross-group audit disclosure error=%v entries=%#v", err, entries)
		}
	})

	t.Run("same group response", func(t *testing.T) {
		sameGroupRecord := crossGroupRecord
		sameGroupRecord.ID = "-100900000002:201:same-group"
		sameGroupRecord.Record.GroupID = requestedGroupID
		service, _ := newConsoleAuditContractService([]ChallengeAuditRecord{sameGroupRecord}, false)

		entries, err := service.ConsoleAudit(context.Background(), requestedGroupID, consoleAuditContractActorID)
		if err != nil || len(entries) != 1 || entries[0].GroupID != requestedGroupID {
			t.Fatalf("same-group audit response entries=%#v error=%v", entries, err)
		}
	})
}

func TestConsoleAuditUndoState(t *testing.T) {
	records := []ChallengeAuditRecord{
		consoleAuditContractRecord(consoleAuditContractGroupID, 301, "-100900000001:301:approved", ChallengeApproved, 301),
		consoleAuditContractRecord(consoleAuditContractGroupID, 302, "-100900000001:302:declined", ChallengeDeclined, 302),
		consoleAuditContractRecord(consoleAuditContractGroupID, 303, "-100900000001:303:expired", ChallengeExpired, 303),
		consoleAuditContractRecord(consoleAuditContractGroupID, 304, "-100900000001:304:superseded", ChallengeSuperseded, 304),
		consoleAuditContractRecord(consoleAuditContractGroupID, 305, "-100900000001:305:pending", ChallengePending, 305),
		consoleAuditContractRecord(consoleAuditContractGroupID, 306, "-100900000001:306:latest-ban", ChallengeBanned, 306),
		consoleAuditContractRecord(consoleAuditContractGroupID, 307, "-100900000001:307:undo-pending", ChallengeBanned, 307),
		consoleAuditContractRecord(consoleAuditContractGroupID, 308, "-100900000001:308:undo-failed", ChallengeBanned, 308),
	}
	records[6].UndoAction = ChallengeActionPending
	records[7].UndoAction = ChallengeActionFailed
	service, _ := newConsoleAuditContractService(records, false)

	entries, err := service.ConsoleAudit(context.Background(), consoleAuditContractGroupID, consoleAuditContractActorID)
	if err != nil || len(entries) != len(records) {
		t.Fatalf("undo state audit entries=%#v error=%v", entries, err)
	}
	states := make(map[string]ConsoleUndoState, len(entries))
	for _, entry := range entries {
		states[entry.ID] = entry.UndoState
	}
	for _, record := range records[:5] {
		if got, ok := states[record.ID]; !ok || got != ConsoleUndoUnavailable {
			t.Errorf("%s undo state = %q, want %q", record.State, got, ConsoleUndoUnavailable)
		}
	}
	if got := states["-100900000001:306:latest-ban"]; got != ConsoleUndoAvailable {
		t.Errorf("valid latest ban undo state = %q, want %q", got, ConsoleUndoAvailable)
	}
	if got := states["-100900000001:307:undo-pending"]; got != ConsoleUndoPending {
		t.Errorf("pending undo action state = %q, want %q", got, ConsoleUndoPending)
	}
	if got := states["-100900000001:308:undo-failed"]; got != ConsoleUndoFailed {
		t.Errorf("failed undo action state = %q, want %q", got, ConsoleUndoFailed)
	}
}

func consoleAuditContractRecord(
	groupID, userID int64,
	id string,
	state ChallengeState,
	settledAt int64,
) ChallengeAuditRecord {
	return ChallengeAuditRecord{
		ID: id,
		Record: PendingRecord{
			GroupID: groupID,
			UserID:  userID,
			Nonce:   id,
			Epoch:   1,
		},
		State:            state,
		SettledAt:        settledAt,
		SettledBy:        consoleAuditContractActorID,
		SettlementAction: ChallengeActionDone,
	}
}

func newConsoleAuditContractService(
	records []ChallengeAuditRecord,
	allowUndo bool,
) (*Service, *fakeVerifyBot) {
	service := newTestService(&settings.Config{GroupIDs: []int64{consoleAuditContractGroupID}})
	service.timeNow = func() time.Time { return time.Unix(500, 0) }
	gateway := newFakeVerifyBot()
	service.gateway = gateway
	service.stateStore = &consoleAuditContractStore{records: records, allowUndo: allowUndo}
	return service, gateway
}
