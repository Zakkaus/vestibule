package settings

import (
	"errors"
	"fmt"
	"reflect"
	"time"
)

// CommitRegistrations atomically commits owner and runtime-group metadata; it always requires durable storage.
func (s *Store) CommitRegistrations(expectedRevision uint64, next RegistrationState) (CommitResult, error) {
	s.writer.Lock()
	defer s.writer.Unlock()
	if s.path == "" || !s.writable {
		if !s.writable {
			return CommitResult{}, fmt.Errorf("%w: %v", ErrSettingsNotDurable, s.unavailableError())
		}
		return CommitResult{}, ErrSettingsNotDurable
	}
	current := s.snapshot.Load().registration
	if current.Revision != expectedRevision {
		return CommitResult{}, &ConflictError{Expected: expectedRevision, Actual: current.Revision}
	}
	if next.Revision != expectedRevision {
		return CommitResult{}, &ConflictError{Expected: next.Revision, Actual: expectedRevision}
	}
	next = normalizeRegistrationState(next)
	next.Revision = expectedRevision
	if reflect.DeepEqual(current, next) {
		return CommitResult{Revision: current.Revision, Durable: true}, nil
	}
	candidate := cloneSettingsFile(s.state)
	candidate.RegistrationRevision = current.Revision + 1
	candidate.OwnerID = next.OwnerID
	candidate.OwnerClaimNonce = next.OwnerClaimNonce
	candidate.OwnerClaimExpiresAt = next.OwnerClaimExpiresAt
	candidate.RegisteredGroups = append([]RegisteredGroup(nil), next.RegisteredGroups...)
	candidate.EnrollmentNonces = append([]EnrollmentNonce(nil), next.EnrollmentNonces...)
	candidate.PendingRegistrations = append([]PendingRegistration(nil), next.PendingRegistrations...)
	candidate.UnknownGroupLeaves = append([]UnknownGroupLeave(nil), next.UnknownGroupLeaves...)

	keep := make(map[int64]bool, len(s.baseline.Groups)+len(next.RegisteredGroups))
	for _, group := range s.baseline.Groups {
		keep[group.ID] = true
	}
	for _, group := range next.RegisteredGroups {
		keep[group.ID] = true
	}
	for groupID := range candidate.Groups {
		if !keep[groupID] {
			delete(candidate.Groups, groupID)
		}
	}
	snap, err := s.buildSnapshot(candidate)
	if err != nil {
		return CommitResult{}, err
	}
	if err := s.writeState(&candidate); err != nil {
		s.setLastError(err)
		return CommitResult{}, err
	}
	s.state = candidate
	s.snapshot.Store(snap)
	s.setLastError(nil)
	return CommitResult{Revision: candidate.RegistrationRevision, Durable: true}, nil
}

// EnsureOwnerClaim returns the current unexpired claim nonce or durably creates a replacement.
func (s *Store) EnsureOwnerClaim(now time.Time, lifetime time.Duration) (nonce string, created bool, err error) {
	if lifetime <= 0 {
		return "", false, fmt.Errorf("owner claim lifetime must be positive")
	}
	for {
		current := s.Registrations()
		if current.OwnerID != 0 {
			return "", false, nil
		}
		if current.OwnerClaimNonce != "" && now.Unix() < current.OwnerClaimExpiresAt {
			return current.OwnerClaimNonce, false, nil
		}
		nonce, err = randomRegistrationNonce()
		if err != nil {
			return "", false, err
		}
		next := cloneRegistrationState(current)
		next.OwnerClaimNonce = nonce
		next.OwnerClaimExpiresAt = now.Add(lifetime).Unix()
		if _, err = s.CommitRegistrations(current.Revision, next); errors.Is(err, ErrSettingsConflict) {
			continue
		}
		return nonce, err == nil, err
	}
}

// ClaimOwner atomically consumes one unexpired owner claim and binds the first owner.
func (s *Store) ClaimOwner(userID int64, nonce string, now time.Time) error {
	if userID <= 0 {
		return ErrOwnerClaimInvalid
	}
	for {
		current := s.Registrations()
		if current.OwnerID != 0 || current.OwnerClaimNonce == "" ||
			current.OwnerClaimNonce != nonce || !now.Before(time.Unix(current.OwnerClaimExpiresAt, 0)) {
			return ErrOwnerClaimInvalid
		}
		next := cloneRegistrationState(current)
		next.OwnerID = userID
		next.OwnerClaimNonce = ""
		next.OwnerClaimExpiresAt = 0
		if _, err := s.CommitRegistrations(current.Revision, next); errors.Is(err, ErrSettingsConflict) {
			continue
		} else {
			return err
		}
	}
}

// IssueEnrollmentNonce durably creates one owner-authorized, single-use registration capability.
func (s *Store) IssueEnrollmentNonce(ownerID int64, now time.Time, lifetime time.Duration) (EnrollmentNonce, error) {
	if lifetime <= 0 {
		return EnrollmentNonce{}, fmt.Errorf("enrollment lifetime must be positive")
	}
	for {
		current := s.Registrations()
		if current.OwnerID == 0 || current.OwnerID != ownerID {
			return EnrollmentNonce{}, ErrRegistrationOwnerOnly
		}
		nonce, err := randomRegistrationNonce()
		if err != nil {
			return EnrollmentNonce{}, err
		}
		issued := EnrollmentNonce{Nonce: nonce, IssuedBy: ownerID, ExpiresAt: now.Add(lifetime).Unix()}
		next := cloneRegistrationState(current)
		next.EnrollmentNonces = next.EnrollmentNonces[:0]
		for _, existing := range current.EnrollmentNonces {
			if now.Unix() < existing.ExpiresAt {
				next.EnrollmentNonces = append(next.EnrollmentNonces, existing)
			}
		}
		next.EnrollmentNonces = append(next.EnrollmentNonces, issued)
		if _, err = s.CommitRegistrations(current.Revision, next); errors.Is(err, ErrSettingsConflict) {
			continue
		} else if err != nil {
			return EnrollmentNonce{}, err
		}
		return issued, nil
	}
}
