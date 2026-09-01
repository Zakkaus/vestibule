package verification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
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

type latestAuditSettlement struct {
	at    int64
	count int
}

// ConsoleAudit returns terminal challenge decisions without treating settings as audited data.
func (v *Service) ConsoleAudit(ctx context.Context, groupID, actorID int64) ([]ConsoleAuditEntry, error) {
	if groupID == 0 || actorID <= 0 {
		return nil, ErrConsoleAuditInvalid
	}
	records, err := v.loadChallengeAudit(ctx, groupID)
	if err != nil {
		return nil, err
	}
	latest := latestAuditSettlements(records)
	entries := make([]ConsoleAuditEntry, 0, len(records))
	for _, record := range records {
		entries = append(entries, consoleAuditEntry(record, actorID, latest[record.Record.UserID]))
	}
	return entries, nil
}

// UndoConsoleAudit removes only the ban created by the latest challenge decision from this actor.
func (v *Service) UndoConsoleAudit(ctx context.Context, undo ConsoleAuditUndo) (ConsoleAuditEntry, error) {
	if undo.GroupID == 0 || undo.ActorID <= 0 || strings.TrimSpace(undo.ID) == "" || strings.Contains(undo.ID, "/") {
		return ConsoleAuditEntry{}, ErrConsoleAuditInvalid
	}
	store, records, err := v.challengeAudit(ctx, undo.GroupID)
	if err != nil {
		return ConsoleAuditEntry{}, err
	}
	record, found := challengeAuditByID(records, undo.ID)
	if !found {
		return ConsoleAuditEntry{}, ErrConsoleAuditNotFound
	}
	latest := latestAuditSettlements(records)
	entry := consoleAuditEntry(record, undo.ActorID, latest[record.Record.UserID])

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

	entries, err := v.ConsoleAudit(ctx, undo.GroupID, undo.ActorID)
	if err != nil {
		return ConsoleAuditEntry{}, err
	}
	for _, current := range entries {
		if current.ID == undo.ID {
			return current, nil
		}
	}
	return ConsoleAuditEntry{}, ErrConsoleAuditNotFound
}

func (v *Service) challengeAudit(
	ctx context.Context,
	groupID int64,
) (challengeAuditStore, []ChallengeAuditRecord, error) {
	store, ok := v.stateStore.(challengeAuditStore)
	if !ok {
		return nil, nil, ErrConsoleAuditUnavailable
	}
	records, err := store.LoadChallengeAudit(ctx, groupID)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", ErrConsoleAuditUnavailable, err)
	}
	for _, record := range records {
		if record.Record.GroupID != groupID {
			return nil, nil, fmt.Errorf("%w: store returned challenge %s for group %d",
				ErrConsoleAuditUnavailable, record.ID, record.Record.GroupID)
		}
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].SettledAt == records[j].SettledAt {
			return records[i].ID > records[j].ID
		}
		return records[i].SettledAt > records[j].SettledAt
	})
	return store, records, nil
}

func (v *Service) loadChallengeAudit(ctx context.Context, groupID int64) ([]ChallengeAuditRecord, error) {
	_, records, err := v.challengeAudit(ctx, groupID)
	return records, err
}

func latestAuditSettlements(records []ChallengeAuditRecord) map[int64]latestAuditSettlement {
	latest := make(map[int64]latestAuditSettlement)
	for _, record := range records {
		current, exists := latest[record.Record.UserID]
		switch {
		case !exists || record.SettledAt > current.at:
			latest[record.Record.UserID] = latestAuditSettlement{at: record.SettledAt, count: 1}
		case record.SettledAt == current.at:
			current.count++
			latest[record.Record.UserID] = current
		}
	}
	return latest
}

func consoleAuditEntry(
	record ChallengeAuditRecord,
	actorID int64,
	latest latestAuditSettlement,
) ConsoleAuditEntry {
	return ConsoleAuditEntry{
		ID: record.ID, GroupID: record.Record.GroupID, UserID: record.Record.UserID,
		Name: record.Record.Name, State: record.State, Reason: record.Reason,
		SettledAt: time.Unix(record.SettledAt, 0).UTC(), SettledBy: record.SettledBy,
		UndoState: consoleAuditUndoState(record, actorID, latest),
	}
}

func consoleAuditUndoState(
	record ChallengeAuditRecord,
	actorID int64,
	latest latestAuditSettlement,
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
	isLatest := latest.count == 1 && latest.at == record.SettledAt
	if record.State == ChallengeBanned && record.SettledBy == actorID && settlementComplete && isLatest {
		return ConsoleUndoAvailable
	}
	return ConsoleUndoUnavailable
}

func challengeAuditByID(records []ChallengeAuditRecord, id string) (ChallengeAuditRecord, bool) {
	for _, record := range records {
		if record.ID == id {
			return record, true
		}
	}
	return ChallengeAuditRecord{}, false
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
