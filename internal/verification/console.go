package verification

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	ErrConsoleChallengeConflict = errors.New("console challenge is no longer pending")
	ErrConsoleSettlementInvalid = errors.New("invalid console settlement")
	ErrConsoleTargetUnavailable = errors.New("cannot verify target membership")
	ErrConsoleTargetProtected   = errors.New("target is a group administrator")
)

const consoleDeclineReason = "console-decline"

// ConsoleQueueEntry is the live challenge view intentionally exposed to the console adapter.
type ConsoleQueueEntry struct {
	ID        string
	GroupID   int64
	UserID    int64
	Name      string
	State     ChallengeState
	Reason    string
	CreatedAt time.Time
	ExpiresAt time.Time
}

// ConsoleSettlement describes a compare-and-set state transition requested by an authenticated admin.
type ConsoleSettlement struct {
	ID       string
	GroupID  int64
	ActorID  int64
	Expected ChallengeState
	Target   ChallengeState
	Reason   string
}

// ConsoleGroups returns configuration-owned group IDs without exposing settings to HTTP handlers.
func (v *Service) ConsoleGroups() []int64 {
	if v.settings == nil {
		return nil
	}
	return v.settings.ChatIDs()
}

// ConsoleGroupTitle returns a runtime-registered group title when one is known.
func (v *Service) ConsoleGroupTitle(groupID int64) string {
	if v.settings == nil {
		return ""
	}
	return v.settings.ChatTitle(groupID)
}

// ConsoleQueue reads durable pending challenges. Terminal history stays outside the console queue.
func (v *Service) ConsoleQueue(ctx context.Context, groupID int64) ([]ConsoleQueueEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if v.stateUnavailable(v.statePath) {
		return v.consoleQueueMemory(groupID), nil
	}
	records, err := v.stateStore.LoadPending(v.statePath)
	if err != nil {
		return nil, fmt.Errorf("load console queue: %w", err)
	}
	entries := make([]ConsoleQueueEntry, 0, len(records))
	for _, record := range records {
		if record.GroupID == groupID {
			entries = append(entries, consoleEntry(record))
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ExpiresAt.Before(entries[j].ExpiresAt) })
	return entries, nil
}

func (v *Service) consoleQueueMemory(groupID int64) []ConsoleQueueEntry {
	v.mu.Lock()
	defer v.mu.Unlock()
	entries := make([]ConsoleQueueEntry, 0)
	for key, item := range v.pend {
		if key.gid != groupID || item == nil || item.done {
			continue
		}
		entries = append(entries, consoleEntry(pendingRecord(key, item)))
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ExpiresAt.Before(entries[j].ExpiresAt) })
	return entries
}

func consoleEntry(record PendingRecord) ConsoleQueueEntry {
	var createdAt time.Time
	if record.CreatedAt != 0 {
		createdAt = time.Unix(record.CreatedAt, 0)
	}
	return ConsoleQueueEntry{
		ID: challengeConsoleID(record.GroupID, record.UserID, record.Nonce), GroupID: record.GroupID,
		UserID: record.UserID, Name: record.Name, State: ChallengePending, CreatedAt: createdAt,
		ExpiresAt: time.Unix(record.Deadline, 0),
	}
}

// SettleConsole first proves the pending condition still holds durably, then verifies the target
// is not an administrator, claims the exact nonce, and invokes the established settlement paths.
func (v *Service) SettleConsole(ctx context.Context, settlement ConsoleSettlement) (ConsoleQueueEntry, error) {
	ref, err := consoleSettlementRef(settlement)
	if err != nil {
		return ConsoleQueueEntry{}, err
	}
	current, err := v.consolePendingCurrent(ref)
	if err != nil {
		return ConsoleQueueEntry{}, err
	}
	if !current {
		return ConsoleQueueEntry{}, ErrConsoleChallengeConflict
	}
	if err := v.consoleTargetAllowed(ctx, ref.GroupID, ref.UserID); err != nil {
		return ConsoleQueueEntry{}, err
	}
	pending, claimed, err := v.claimPendingNonceAs(ref.GroupID, ref.UserID, ref.Nonce, settlement.Target,
		consoleStoredReason(settlement), settlement.ActorID)
	if err != nil {
		return ConsoleQueueEntry{}, fmt.Errorf("claim console challenge: %w", err)
	}
	if !claimed {
		return ConsoleQueueEntry{}, ErrConsoleChallengeConflict
	}
	entry := consoleEntry(pendingRecord(pkey{gid: ref.GroupID, uid: ref.UserID}, pending))
	entry.State, entry.Reason = settlement.Target, consoleResponseReason(settlement)
	v.executeConsoleSettlement(ctx, ref, pending, settlement.Target)
	return entry, nil
}

func consoleSettlementRef(settlement ConsoleSettlement) (PendingRef, error) {
	if settlement.GroupID == 0 || settlement.ActorID <= 0 || settlement.Expected != ChallengePending ||
		!validConsoleTarget(settlement.Target) || !validConsoleReason(settlement.Target, settlement.Reason) {
		return PendingRef{}, ErrConsoleSettlementInvalid
	}
	parts := strings.Split(settlement.ID, ":")
	if len(parts) != 3 || parts[2] == "" || len(parts[2]) > 64 {
		return PendingRef{}, ErrConsoleSettlementInvalid
	}
	groupID, groupErr := strconv.ParseInt(parts[0], 10, 64)
	userID, userErr := strconv.ParseInt(parts[1], 10, 64)
	if groupErr != nil || userErr != nil || groupID != settlement.GroupID || userID <= 0 {
		return PendingRef{}, ErrConsoleSettlementInvalid
	}
	return PendingRef{GroupID: groupID, UserID: userID, Nonce: parts[2]}, nil
}

func validConsoleTarget(target ChallengeState) bool {
	return target == ChallengeApproved || target == ChallengeDeclined || target == ChallengeBanned
}

func validConsoleReason(target ChallengeState, reason string) bool {
	if target != ChallengeDeclined {
		return reason == ""
	}
	return reason == "" || reason == "wrong_answer" || reason == "rejected" || reason == "external_unmet"
}

func consoleStoredReason(settlement ConsoleSettlement) string {
	if settlement.Target == ChallengeDeclined && settlement.Reason != "" {
		return settlement.Reason
	}
	if settlement.Target == ChallengeDeclined {
		return "rejected"
	}
	return ""
}

func consoleResponseReason(settlement ConsoleSettlement) string {
	if settlement.Target == ChallengeDeclined {
		return consoleStoredReason(settlement)
	}
	return ""
}

func (v *Service) consoleTargetAllowed(ctx context.Context, groupID, userID int64) error {
	if v.gateway == nil {
		return ErrConsoleTargetUnavailable
	}
	member, err := v.gateway.Member(ctx, groupID, userID)
	if err != nil || member == nil {
		return ErrConsoleTargetUnavailable
	}
	switch member.MemberStatus() {
	case MemberStatusCreator, MemberStatusAdministrator:
		return ErrConsoleTargetProtected
	default:
		return nil
	}
}

func (v *Service) executeConsoleSettlement(ctx context.Context, ref PendingRef, item *pending, target ChallengeState) {
	switch target {
	case ChallengeApproved:
		_ = v.executeApprove(ctx, v.gateway, ref.GroupID, ref.UserID, item)
	case ChallengeDeclined:
		_, _ = v.finishDecline(ctx, v.gateway, ref.GroupID, ref.UserID, item, consoleDeclineReason)
	case ChallengeBanned:
		_ = v.executeBan(ctx, v.gateway, ref.GroupID, ref.UserID, item)
	}
}

func (v *Service) consolePendingCurrent(ref PendingRef) (bool, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	key := pkey{gid: ref.GroupID, uid: ref.UserID}
	item := v.pend[key]
	if item == nil || item.done || item.nonce != ref.Nonce {
		return false, nil
	}
	if v.stateUnavailable(v.statePath) {
		return true, nil
	}
	changed, err := v.updatePendingLocked(key, item, item.epoch)
	if err != nil {
		return false, fmt.Errorf("check console challenge: %w", err)
	}
	if !changed {
		v.forgetPendingLocked(key, item)
	}
	return changed, nil
}

func challengeConsoleID(groupID, userID int64, nonce string) string {
	return strconv.FormatInt(groupID, 10) + ":" + strconv.FormatInt(userID, 10) + ":" + nonce
}
