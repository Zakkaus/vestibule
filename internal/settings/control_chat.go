package settings

import (
	"errors"
	"fmt"
)

var ErrControlChatInvalid = errors.New("invalid control chat")
var ErrControlChatConflict = errors.New("control chat already assigned")

// ControlChatConflictError identifies the group already using a control chat.
type ControlChatConflictError struct {
	ChatID       int64
	OtherGroupID int64
}

func (e *ControlChatConflictError) Error() string {
	return fmt.Sprintf("control chat %d is already used by group %d", e.ChatID, e.OtherGroupID)
}

func (e *ControlChatConflictError) Is(target error) bool { return target == ErrControlChatConflict }

type controlChatMembershipCheck struct {
	check func(int64, int64) error
}

type controlChatApproval struct {
	chatID, previousID int64
	checker            *controlChatMembershipCheck
}

// ControlGroup resolves a control chat without granting it protected-group status.
func (s *Store) ControlGroup(chatID int64) (GroupView, bool) {
	if chatID == 0 {
		return GroupView{}, false
	}
	snapshot := s.snapshot.Load()
	for _, id := range snapshot.groupIDs {
		group := snapshot.groups[id]
		if group.controlChatID.Value == chatID {
			return GroupView{group: group}, true
		}
	}
	return GroupView{}, false
}

// SetControlChatMembership installs live chat-type, bot-membership and actor-admin checks.
func (s *Store) SetControlChatMembership(check func(int64, int64) error) {
	if check == nil {
		s.controlChatMembership.Store(nil)
		return
	}
	s.controlChatMembership.Store(&controlChatMembershipCheck{check: check})
}

func (s *Store) prepareControlChatWrite(groupID int64, revision uint64, next GroupOverrides, actorID int64) (controlChatApproval, error) {
	group, ok := s.snapshot.Load().groups[groupID]
	if !ok {
		return controlChatApproval{}, fmt.Errorf("%w: %d", ErrUnknownGroup, groupID)
	}
	if group.revision != revision {
		return controlChatApproval{}, &ConflictError{GroupID: groupID, Expected: revision, Actual: group.revision}
	}
	id := resolve(next.ControlChatID, group.baseline.ControlChatID).Value
	if err := s.validateControlChatCandidate(group.id, id); err != nil {
		return controlChatApproval{}, err
	}
	approval := controlChatApproval{chatID: id, previousID: group.controlChatID.Value}
	if id == 0 || id == approval.previousID {
		return approval, nil
	}
	approval.checker = s.controlChatMembership.Load()
	if actorID <= 0 || approval.checker == nil {
		return controlChatApproval{}, fmt.Errorf("%w: control-chat authorization unavailable", ErrControlChatInvalid)
	}
	if err := approval.checker.check(id, actorID); err != nil {
		return controlChatApproval{}, fmt.Errorf("%w: chat %d: %v", ErrControlChatInvalid, id, err)
	}
	return approval, nil
}

func (s *Store) validateControlChatCandidate(groupID, chatID int64) error {
	if chatID == groupID || chatID > 0 {
		return fmt.Errorf("%w: group %d cannot use chat %d", ErrControlChatInvalid, groupID, chatID)
	}
	if other, ok := s.ControlGroup(chatID); ok && other.ID() != groupID {
		return &ControlChatConflictError{ChatID: chatID, OtherGroupID: other.ID()}
	}
	return nil
}

func (s *Store) validateControlChatWrite(before *effectiveGroup, next GroupOverrides, approval controlChatApproval) error {
	id := resolve(next.ControlChatID, before.baseline.ControlChatID).Value
	if err := s.validateControlChatCandidate(before.id, id); err != nil {
		return err
	}
	if id == 0 || id == before.controlChatID.Value {
		return nil
	}
	if approval.chatID != id || approval.previousID != before.controlChatID.Value ||
		approval.checker == nil || approval.checker != s.controlChatMembership.Load() {
		return fmt.Errorf("%w: control-chat authorization changed", ErrControlChatInvalid)
	}
	return nil
}

func validateControlChats(groups map[int64]*effectiveGroup, order []int64) error {
	seen := make(map[int64]int64)
	for _, id := range order {
		chat := groups[id].controlChatID.Value
		if chat == 0 {
			continue
		}
		if chat == id || chat > 0 {
			return fmt.Errorf("%w: group %d cannot control itself", ErrControlChatInvalid, id)
		}
		if other, ok := seen[chat]; ok {
			return &ControlChatConflictError{ChatID: chat, OtherGroupID: other}
		}
		seen[chat] = id
	}
	return nil
}
