// Package database owns durable state adapters. The verification adapter remains JSON-backed until
// the phase-three database migration.
package database

import (
	"fmt"

	"github.com/Zakkaus/vestibule/internal/store"
	"github.com/Zakkaus/vestibule/internal/verification"
)

// VerificationJSONStore preserves the legacy atomic JSON write and corruption-recovery behavior.
type VerificationJSONStore struct{}

var _ verification.Store = (*VerificationJSONStore)(nil)

func NewVerificationJSONStore() *VerificationJSONStore { return &VerificationJSONStore{} }

func (s *VerificationJSONStore) LoadPending(path string) ([]verification.PendingRecord, error) {
	var records []verification.PendingRecord
	if err := store.Load(path, &records); err != nil {
		return nil, stateLoadError(err)
	}
	return records, nil
}

func (s *VerificationJSONStore) SavePending(path string, snapshot func() []verification.PendingRecord) error {
	return store.Save(path, func() any { return snapshot() })
}

func (s *VerificationJSONStore) LoadFailures(path string) ([]verification.FailureRecord, error) {
	var records []verification.FailureRecord
	if err := store.Load(path, &records); err != nil {
		return nil, stateLoadError(err)
	}
	return records, nil
}

func (s *VerificationJSONStore) SaveFailures(path string, snapshot func() []verification.FailureRecord) error {
	return store.Save(path, func() any { return snapshot() })
}

func (s *VerificationJSONStore) LoadAgents(path string) (verification.AgentTally, error) {
	var tally verification.AgentTally
	if err := store.Load(path, &tally); err != nil {
		return verification.AgentTally{}, stateLoadError(err)
	}
	return tally, nil
}

func (s *VerificationJSONStore) SaveAgents(path string, snapshot func() verification.AgentTally) error {
	return store.Save(path, func() any { return snapshot() })
}

func (s *VerificationJSONStore) LoadHeartbeat(path string) (verification.HeartbeatRecord, error) {
	var heartbeat verification.HeartbeatRecord
	if err := store.Load(path, &heartbeat); err != nil {
		return verification.HeartbeatRecord{}, stateLoadError(err)
	}
	return heartbeat, nil
}

func (s *VerificationJSONStore) SaveHeartbeat(path string, heartbeat verification.HeartbeatRecord) error {
	return store.Write(path, heartbeat)
}

func stateLoadError(err error) error {
	if store.ReadFailed(err) {
		return fmt.Errorf("%w: %v", verification.ErrStoreReadOnly, err)
	}
	return err
}
