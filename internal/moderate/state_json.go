package moderate

import (
	"fmt"

	"github.com/Zakkaus/vestibule/internal/store"
)

type warningJSONStore struct {
	path string
}

var _ WarningStore = (*warningJSONStore)(nil)

func newWarningJSONStore(path string) WarningStore {
	if path == "" {
		return nil
	}
	return &warningJSONStore{path: path}
}

func (s *warningJSONStore) LoadWarnings() ([]WarningRecord, error) {
	if s.path == "" {
		return nil, nil
	}
	var records []WarningRecord
	if err := store.Load(s.path, &records); err != nil {
		if store.ReadFailed(err) {
			return nil, fmt.Errorf("%w: %v", ErrWarningStoreReadOnly, err)
		}
		return nil, err
	}
	return records, nil
}

func (s *warningJSONStore) SaveWarnings(snapshot func() []WarningRecord) error {
	if s.path == "" {
		return nil
	}
	return store.Save(s.path, func() any { return snapshot() })
}
