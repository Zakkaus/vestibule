package database

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/Zakkaus/vestibule/internal/rules"
)

// RuleStore persists ordered rule collections for the console and runtime services.
type RuleStore struct {
	db *Database
}

// NewRuleStore binds rule collections to the shared database.
func NewRuleStore(db *Database) *RuleStore {
	return &RuleStore{db: db}
}

// ListRules returns a chat's rules ordered by collection and ordinal. A blank collection lists all collections.
func (s *RuleStore) ListRules(ctx context.Context, chatID int64, collection string) ([]rules.Record, error) {
	if chatID == 0 {
		return nil, fmt.Errorf("%w: chat ID is required", rules.ErrRuleInvalid)
	}
	records, err := s.listRules(ctx, chatID, collection)
	if err != nil {
		return nil, fmt.Errorf("list rules for chat %d: %w", chatID, err)
	}
	if err = validateLoadedRuleOrder(records); err != nil {
		return nil, fmt.Errorf("list rules for chat %d: %w", chatID, err)
	}
	return records, nil
}

func (s *RuleStore) listRules(ctx context.Context, chatID int64, collection string) ([]rules.Record, error) {
	query := `
		SELECT id, chat_id, collection, ordinal, enabled, definition
		  FROM rule
		 WHERE chat_id=$1
		 ORDER BY collection, ordinal, id`
	args := []any{chatID}
	if collection != "" {
		query = `
			SELECT id, chat_id, collection, ordinal, enabled, definition
			  FROM rule
			 WHERE chat_id=$1 AND collection=$2
			 ORDER BY collection, ordinal, id`
		args = append(args, collection)
	}
	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]rules.Record, 0)
	for rows.Next() {
		record, scanErr := scanRule(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func scanRule(row interface{ Scan(...any) error }) (rules.Record, error) {
	var record rules.Record
	var definition string
	if err := row.Scan(
		&record.ID, &record.ChatID, &record.Collection, &record.Ordinal, &record.Enabled, &definition,
	); err != nil {
		return rules.Record{}, err
	}
	record.Definition = json.RawMessage(definition)
	if err := validateRuleRecord(record); err != nil {
		return rules.Record{}, fmt.Errorf("stored rule %q: %w", record.ID, err)
	}
	return record, nil
}

// ReplaceRules atomically replaces one complete collection when expected is still current.
func (s *RuleStore) ReplaceRules(
	ctx context.Context,
	chatID int64,
	collection string,
	expected []rules.Record,
	next []rules.Record,
) ([]rules.Record, bool, error) {
	if err := validateRuleSequence(chatID, collection, expected); err != nil {
		return nil, false, err
	}
	if err := validateRuleSequence(chatID, collection, next); err != nil {
		return nil, false, err
	}
	var result []rules.Record
	changed := false
	err := s.db.DoTxn(ctx, nil, func(txCtx context.Context) error {
		var replaceErr error
		result, changed, replaceErr = s.replaceRules(txCtx, chatID, collection, expected, next)
		return replaceErr
	})
	if err != nil {
		return nil, false, fmt.Errorf("replace %q rules for chat %d: %w", collection, chatID, err)
	}
	return result, changed, nil
}

func (s *RuleStore) replaceRules(
	ctx context.Context,
	chatID int64,
	collection string,
	expected []rules.Record,
	next []rules.Record,
) ([]rules.Record, bool, error) {
	if err := s.lockRuleChat(ctx, chatID); err != nil {
		return nil, false, err
	}
	current, err := s.listRules(ctx, chatID, collection)
	if err != nil {
		return nil, false, err
	}
	if sameRuleSequence(current, next) {
		return cloneRuleRecords(current), false, nil
	}
	if !sameRuleSequence(current, expected) {
		return nil, false, rules.ErrRuleConflict
	}
	if _, err = s.db.Exec(ctx, "DELETE FROM rule WHERE chat_id=$1 AND collection=$2", chatID, collection); err != nil {
		return nil, false, err
	}
	for _, record := range next {
		if _, err = s.db.Exec(ctx, `
			INSERT INTO rule (id, chat_id, collection, ordinal, enabled, definition)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			record.ID, record.ChatID, record.Collection, record.Ordinal, record.Enabled, string(record.Definition)); err != nil {
			return nil, false, err
		}
	}
	return cloneRuleRecords(next), true, nil
}

// UpdateRule conditionally replaces one row without changing its collection or ordinal.
func (s *RuleStore) UpdateRule(
	ctx context.Context,
	chatID int64,
	expected rules.Record,
	next rules.Record,
) (rules.Record, bool, error) {
	if err := validateRuleRecord(expected); err != nil {
		return rules.Record{}, false, err
	}
	if err := validateRuleRecord(next); err != nil {
		return rules.Record{}, false, err
	}
	if chatID != expected.ChatID || !sameRuleIdentity(expected, next) {
		return rules.Record{}, false, fmt.Errorf("%w: rule identity changed", rules.ErrRuleInvalid)
	}
	var result rules.Record
	changed := false
	err := s.db.DoTxn(ctx, nil, func(txCtx context.Context) error {
		var updateErr error
		result, changed, updateErr = s.updateRule(txCtx, expected, next)
		return updateErr
	})
	if err != nil {
		return rules.Record{}, false, fmt.Errorf("update rule %q for chat %d: %w", expected.ID, chatID, err)
	}
	return result, changed, nil
}

func (s *RuleStore) updateRule(ctx context.Context, expected, next rules.Record) (rules.Record, bool, error) {
	if err := s.lockRuleChat(ctx, expected.ChatID); err != nil {
		return rules.Record{}, false, err
	}
	result, err := s.db.Exec(ctx, `
		UPDATE rule
		   SET enabled=$1, definition=$2
		 WHERE id=$3 AND chat_id=$4 AND collection=$5 AND ordinal=$6
		   AND enabled=$7 AND definition=$8
		   AND (enabled<>$1 OR definition<>$2)`,
		next.Enabled, string(next.Definition), expected.ID, expected.ChatID, expected.Collection,
		expected.Ordinal, expected.Enabled, string(expected.Definition))
	if err != nil {
		return rules.Record{}, false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return rules.Record{}, false, err
	}
	if affected > 1 {
		return rules.Record{}, false, fmt.Errorf("updated %d rows for rule %q", affected, expected.ID)
	}
	if affected == 1 {
		return cloneRuleRecord(next), true, nil
	}
	current, err := s.rule(ctx, expected.ChatID, expected.ID)
	if err != nil {
		return rules.Record{}, false, err
	}
	if sameRule(current, next) {
		return cloneRuleRecord(current), false, nil
	}
	return rules.Record{}, false, rules.ErrRuleConflict
}

func (s *RuleStore) rule(ctx context.Context, chatID int64, ruleID string) (rules.Record, error) {
	record, err := scanRule(s.db.QueryRow(ctx, `
		SELECT id, chat_id, collection, ordinal, enabled, definition
		  FROM rule
		 WHERE id=$1 AND chat_id=$2`, ruleID, chatID))
	if err == sql.ErrNoRows {
		return rules.Record{}, rules.ErrRuleNotFound
	}
	return record, err
}

func (s *RuleStore) lockRuleChat(ctx context.Context, chatID int64) error {
	result, err := s.db.Exec(ctx, "UPDATE chat SET title=title WHERE id=$1", chatID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return rules.ErrRuleChatNotFound
	}
	if affected != 1 {
		return fmt.Errorf("locked %d chat rows for %d", affected, chatID)
	}
	return nil
}

func validateRuleSequence(chatID int64, collection string, records []rules.Record) error {
	if chatID == 0 || collection == "" {
		return fmt.Errorf("%w: chat ID and collection are required", rules.ErrRuleInvalid)
	}
	seen := make(map[string]struct{}, len(records))
	for index, record := range records {
		if err := validateRuleRecord(record); err != nil {
			return err
		}
		if record.ChatID != chatID || record.Collection != collection || record.Ordinal != index {
			return fmt.Errorf("%w: rule %q has mismatched identity or ordinal", rules.ErrRuleInvalid, record.ID)
		}
		if _, exists := seen[record.ID]; exists {
			return fmt.Errorf("%w: duplicate rule ID %q", rules.ErrRuleInvalid, record.ID)
		}
		seen[record.ID] = struct{}{}
	}
	return nil
}

func validateRuleRecord(record rules.Record) error {
	if record.ID == "" || record.ChatID == 0 || record.Collection == "" || record.Ordinal < 0 {
		return fmt.Errorf("%w: incomplete rule identity", rules.ErrRuleInvalid)
	}
	if !json.Valid(record.Definition) {
		return fmt.Errorf("%w: rule %q definition is not JSON", rules.ErrRuleInvalid, record.ID)
	}
	return nil
}

func validateLoadedRuleOrder(records []rules.Record) error {
	nextOrdinal := make(map[string]int)
	for _, record := range records {
		if record.Ordinal != nextOrdinal[record.Collection] {
			return fmt.Errorf("%w: collection %q has ordinal %d after %d",
				rules.ErrRuleInvalid, record.Collection, record.Ordinal, nextOrdinal[record.Collection]-1)
		}
		nextOrdinal[record.Collection]++
	}
	return nil
}

func sameRuleIdentity(left, right rules.Record) bool {
	return left.ID == right.ID && left.ChatID == right.ChatID && left.Collection == right.Collection &&
		left.Ordinal == right.Ordinal
}

func sameRule(left, right rules.Record) bool {
	return sameRuleIdentity(left, right) && left.Enabled == right.Enabled &&
		bytes.Equal(left.Definition, right.Definition)
}

func sameRuleSequence(left, right []rules.Record) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if !sameRule(left[index], right[index]) {
			return false
		}
	}
	return true
}

func cloneRuleRecord(record rules.Record) rules.Record {
	record.Definition = append(json.RawMessage(nil), record.Definition...)
	return record
}

func cloneRuleRecords(records []rules.Record) []rules.Record {
	cloned := make([]rules.Record, len(records))
	for index, record := range records {
		cloned[index] = cloneRuleRecord(record)
	}
	return cloned
}
