package settings

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

type legacyMigration struct {
	migrateAntispam     bool
	write               bool
	preMigrationSource  []byte
	preMigrationVersion int
}

// NewStore loads legacy state and owns the immutable settings snapshot.
func NewStore(path string, baseline SettingsBaseline, repository Repository) (*Store, error) {
	s, err := initializeStore(path, baseline, repository)
	if err != nil {
		return nil, err
	}
	migration := s.loadLegacySettings()
	s.migrateLegacyAntispam(&migration)
	s.persistLegacyMigration(migration)
	if err := s.loadRepositorySettings(); err != nil {
		return nil, err
	}
	return s.initializeSnapshot()
}

func initializeStore(path string, baseline SettingsBaseline, repository Repository) (*Store, error) {
	baseline = cloneSettingsBaseline(baseline)
	normalizeBaselineLanguages(&baseline)
	if err := validateBaseline(baseline); err != nil {
		return nil, err
	}
	s := &Store{
		path:         path,
		baseline:     baseline,
		baselineByID: make(map[int64]GroupBaseline, len(baseline.Groups)),
		writable:     true,
		repository:   repository,
	}
	for _, group := range baseline.Groups {
		s.baselineByID[group.ID] = group
	}
	s.state = s.newState()
	return s, nil
}

func (s *Store) loadLegacySettings() legacyMigration {
	migration := legacyMigration{migrateAntispam: s.path != ""}
	if s.path == "" {
		return migration
	}
	exists := false
	if _, err := os.Stat(s.path); err == nil {
		exists = true
	} else if !os.IsNotExist(err) {
		exists = true
	}
	if !exists {
		return migration
	}
	migration.migrateAntispam = false
	source, err := os.ReadFile(s.path)
	if err != nil {
		s.writable = false
		s.setLastError(fmt.Errorf("%w: read legacy settings: %v", ErrSettingsUnavailable, err))
		return migration
	}
	var version struct {
		Version int `json:"version"`
	}
	if err = decodeYAML(source, &version); err != nil {
		s.writable = false
		s.setLastError(fmt.Errorf("%w: parse legacy settings: %v", ErrSettingsUnavailable, err))
		return migration
	}
	if version.Version > SettingsSchemaVersion {
		s.writable = false
		s.setLastError(fmt.Errorf("%w: schema version %d is newer than supported version %d", ErrSettingsUnavailable, version.Version, SettingsSchemaVersion))
		return migration
	}
	migration.migrateAntispam = version.Version < SettingsSchemaVersion
	upgraded, err := upgradeSettingsFile(s.path, s.baselineIDs(), true)
	if err != nil {
		s.writable = false
		migration.migrateAntispam = false
		s.setLastError(fmt.Errorf("%w: upgrade legacy settings: %v", ErrSettingsUnavailable, err))
		return migration
	}
	migration.preMigrationSource = source
	migration.preMigrationVersion = version.Version
	migration.write = true
	s.state = upgraded
	return migration
}

func (s *Store) baselineIDs() []int64 {
	chatIDs := make([]int64, 0, len(s.baseline.Groups))
	for _, group := range s.baseline.Groups {
		chatIDs = append(chatIDs, group.ID)
	}
	return chatIDs
}

func (s *Store) migrateLegacyAntispam(migration *legacyMigration) {
	if !s.writable || !migration.migrateAntispam {
		return
	}
	legacy, present, err := loadLegacyAntispam(filepath.Join(filepath.Dir(s.path), "antispam.json"))
	if err != nil {
		s.writable = false
		s.setLastError(fmt.Errorf("%w: legacy antispam migration: %v", ErrSettingsUnavailable, err))
		return
	}
	if present {
		s.state = s.migrateLegacyAntispamState(s.state, legacy)
		migration.write = true
	}
}

func (s *Store) persistLegacyMigration(migration legacyMigration) {
	if !s.writable || !migration.write {
		return
	}
	if migration.preMigrationSource != nil {
		backupPath := fmt.Sprintf("%s.v%d.bak", s.path, migration.preMigrationVersion)
		if err := os.WriteFile(backupPath, migration.preMigrationSource, 0o600); err != nil {
			log.Printf("ERROR settings migration: could not back up schema-v%d state %s to %s: %v",
				migration.preMigrationVersion, s.path, backupPath, err)
		}
	}
	if err := s.writeState(&s.state); err != nil {
		s.setLastError(err)
	} else {
		s.setLastError(nil)
	}
}

func (s *Store) loadRepositorySettings() error {
	if s.repository == nil {
		return nil
	}
	seeds := make([]Record, 0, len(s.baseline.Groups)+len(s.state.RegisteredGroups))
	seen := make(map[int64]bool, cap(seeds))
	addSeed := func(chatID int64) {
		if seen[chatID] {
			return
		}
		seen[chatID] = true
		record := s.state.Groups[chatID]
		seeds = append(seeds, Record{ChatID: chatID, Revision: record.Revision, Overrides: record.GroupOverrides})
	}
	for _, group := range s.baseline.Groups {
		addSeed(group.ID)
	}
	for _, group := range s.state.RegisteredGroups {
		addSeed(group.ID)
	}
	if err := s.repository.SeedSettings(seeds); err != nil {
		return fmt.Errorf("seed settings repository: %w", err)
	}
	records, err := s.repository.LoadSettings()
	if err != nil {
		return fmt.Errorf("load settings repository: %w", err)
	}
	s.state.Groups = make(map[int64]groupRecord, len(records))
	for _, record := range records {
		s.state.Groups[record.ChatID] = groupRecord{Revision: record.Revision, GroupOverrides: record.Overrides}
	}
	return nil
}

func (s *Store) initializeSnapshot() (*Store, error) {
	snap, err := s.buildSnapshot(s.state)
	if err == nil {
		s.snapshot.Store(snap)
		return s, nil
	}
	reconciled, adjustments := s.reconcileWithBaseline(s.state)
	if len(adjustments) > 0 {
		if snap, err = s.buildSnapshot(reconciled); err == nil {
			s.writable = false
			s.state = reconciled
			s.setLastError(fmt.Errorf("%w: reconciled with config: %s", ErrSettingsUnavailable, strings.Join(adjustments, "; ")))
			s.snapshot.Store(snap)
			return s, nil
		}
	}
	s.writable = false
	s.setLastError(fmt.Errorf("%w: %v", ErrSettingsUnavailable, err))
	s.state = s.newState()
	snap, _ = s.buildSnapshot(s.state)
	s.snapshot.Store(snap)
	return s, nil
}

func (s *Store) newState() settingsFile {
	return settingsFile{
		Version:              SettingsSchemaVersion,
		RegisteredGroups:     []RegisteredGroup{},
		EnrollmentNonces:     []EnrollmentNonce{},
		PendingRegistrations: []PendingRegistration{},
		UnknownGroupLeaves:   []UnknownGroupLeave{},
		Groups:               make(map[int64]groupRecord),
	}
}

func loadLegacyAntispam(path string) (legacyAntispamState, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return legacyAntispamState{}, false, nil
		}
		return legacyAntispamState{}, false, err
	}
	var legacy legacyAntispamState
	if err := json.Unmarshal(data, &legacy); err != nil {
		return legacyAntispamState{}, false, err
	}
	return legacy, true, nil
}

func (s *Store) migrateLegacyAntispamState(state settingsFile, legacy legacyAntispamState) settingsFile {
	migrated := cloneSettingsFile(state)
	apply := func(groupID int64) {
		record := migrated.Groups[groupID]
		changed := false
		if record.AntispamEnabled == nil {
			record.AntispamEnabled = new(legacy.Enabled)
			changed = true
		}
		if record.ChannelWhitelist == nil {
			record.ChannelWhitelist = cloneSlicePtr(&legacy.Whitelist, cloneInt64s)
			changed = true
		}
		if changed && record.Revision == 0 {
			record.Revision = 1
		}
		migrated.Groups[groupID] = record
	}
	for _, group := range s.baseline.Groups {
		apply(group.ID)
	}
	for _, group := range migrated.RegisteredGroups {
		apply(group.ID)
	}
	return migrated
}
