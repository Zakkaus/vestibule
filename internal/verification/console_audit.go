package verification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrConsoleAuditInvalid     = errors.New("invalid console audit request")
	ErrConsoleAuditUnavailable = errors.New("console audit is unavailable")
	ErrConsoleAuditNotFound    = errors.New("console audit entry was not found")
	ErrConsoleAuditNotUndoable = errors.New("console audit entry cannot be undone by this actor")
	ErrConsoleAuditConflict    = errors.New("console audit entry changed before undo")
)

// ConsoleAudit returns terminal challenge decisions without treating settings as audited data.
func (v *Service) ConsoleAudit(ctx context.Context, groupID, actorID int64, page AuditPageRequest) ([]ConsoleAuditEntry, error) {
	if groupID == 0 || actorID <= 0 {
		return nil, ErrConsoleAuditInvalid
	}
	_, records, err := v.challengeAudit(ctx, groupID, page)
	if err != nil {
		return nil, err
	}
	entries := make([]ConsoleAuditEntry, 0, len(records))
	for _, record := range records {
		entries = append(entries, consoleAuditEntry(record, actorID))
	}
	return entries, nil
}

// UndoConsoleAudit removes only the ban created by the latest challenge decision from this actor.
func (v *Service) UndoConsoleAudit(ctx context.Context, undo ConsoleAuditUndo) (ConsoleAuditEntry, error) {
	if undo.GroupID == 0 || undo.ActorID <= 0 || strings.TrimSpace(undo.ID) == "" || strings.Contains(undo.ID, "/") {
		return ConsoleAuditEntry{}, ErrConsoleAuditInvalid
	}
	store, ok := v.stateStore.(challengeAuditStore)
	if !ok {
		return ConsoleAuditEntry{}, ErrConsoleAuditUnavailable
	}
	record, err := loadAuditTarget(ctx, store, undo.GroupID, undo.ID)
	if err != nil {
		return ConsoleAuditEntry{}, err
	}
	entry := consoleAuditEntry(record, undo.ActorID)

	// Telegram exposes the current ban but not who placed it. Letting any administrator undo it
	// repeats the previous generation's dangerous unban: one administrator silently overrules
	// another. Requiring settled_by to match and this to remain the latest recorded decision is
	// narrower. Refusing every unban would protect out-of-band re-bans too, but would make the
	// planned undo unusable because Telegram provides no provenance for that stricter proof.
	if entry.UndoState != ConsoleUndoAvailable {
		return ConsoleAuditEntry{}, ErrConsoleAuditNotUndoable
	}
	action, err := v.newUndoBanAction(record)
	if err != nil {
		return ConsoleAuditEntry{}, fmt.Errorf("prepare challenge audit undo: %w", err)
	}
	changed, err := store.EnqueueChallengeUndo(ctx, record, action)
	if err != nil {
		return ConsoleAuditEntry{}, fmt.Errorf("enqueue challenge audit undo: %w", err)
	}
	if !changed {
		return ConsoleAuditEntry{}, ErrConsoleAuditConflict
	}
	v.executePendingAction(ctx, v.gateway, v.actionOwner, PendingAction{ActionIntent: action})

	record, err = loadAuditTarget(ctx, store, undo.GroupID, undo.ID)
	if err != nil {
		return ConsoleAuditEntry{}, err
	}
	return consoleAuditEntry(record, undo.ActorID), nil
}

func (v *Service) challengeAudit(
	ctx context.Context,
	groupID int64, page AuditPageRequest,
) (challengeAuditStore, []ChallengeAuditRecord, error) {
	store, ok := v.stateStore.(challengeAuditStore)
	if !ok {
		return nil, nil, ErrConsoleAuditUnavailable
	}
	if _, _, _, err := page.Boundary(groupID); err != nil {
		return nil, nil, err
	}
	records, err := store.LoadChallengeAudit(ctx, groupID, page)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrConsoleAuditUnavailable, err)
	}
	for _, record := range records {
		if record.Record.GroupID != groupID {
			return nil, nil, fmt.Errorf("%w: store returned challenge %s for group %d",
				ErrConsoleAuditUnavailable, record.ID, record.Record.GroupID)
		}
	}
	return store, records, nil
}

func loadAuditTarget(ctx context.Context, store challengeAuditStore, groupID int64, id string) (ChallengeAuditRecord, error) {
	record, found, err := store.LoadChallengeAuditByID(ctx, groupID, id)
	if err != nil {
		return ChallengeAuditRecord{}, fmt.Errorf("%w: %v", ErrConsoleAuditUnavailable, err)
	}
	if !found {
		return ChallengeAuditRecord{}, ErrConsoleAuditNotFound
	}
	if record.Record.GroupID != groupID {
		return ChallengeAuditRecord{}, ErrConsoleAuditUnavailable
	}
	return record, nil
}

func consoleAuditEntry(
	record ChallengeAuditRecord,
	actorID int64,
) ConsoleAuditEntry {
	return ConsoleAuditEntry{
		ID: record.ID, GroupID: record.Record.GroupID, UserID: record.Record.UserID,
		Name: record.Record.Name, State: record.State, Reason: record.Reason,
		SettledAt: time.Unix(record.SettledAt, 0).UTC(), SettledBy: record.SettledBy,
		UndoState: consoleAuditUndoState(record, actorID),
	}
}

func consoleAuditUndoState(
	record ChallengeAuditRecord,
	actorID int64,
) ConsoleUndoState {
	switch record.UndoAction {
	case ChallengeActionPending:
		return ConsoleUndoPending
	case ChallengeActionDone:
		return ConsoleUndoCompleted
	case ChallengeActionFailed:
		return ConsoleUndoFailed
	}
	settlementComplete := record.SettlementAction == ChallengeActionNone ||
		record.SettlementAction == ChallengeActionDone
	if record.State == ChallengeBanned && record.SettledBy == actorID && settlementComplete && record.Latest {
		return ConsoleUndoAvailable
	}
	return ConsoleUndoUnavailable
}

func (v *Service) newUndoBanAction(record ChallengeAuditRecord) (ActionIntent, error) {
	payload, err := json.Marshal(undoBanActionPayload{
		ChatID: record.Record.GroupID,
		UserID: record.Record.UserID,
	})
	if err != nil {
		return ActionIntent{}, fmt.Errorf("encode ban undo: %w", err)
	}
	now := v.wallNow()
	return ActionIntent{
		ID: "undo:" + record.ID + ":ban", Kind: actionUndoBan, Payload: string(payload),
		NextTryAt: now.Unix(), ClaimOwner: v.actionOwner, ClaimUntil: now.Add(actionClaimLease).Unix(),
	}, nil
}
