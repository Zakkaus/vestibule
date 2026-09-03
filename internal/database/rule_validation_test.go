package database

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Zakkaus/vestibule/internal/rules"
)

func TestUpdatingARuleRejectsAnEmptyIDCollectionOrNegativeOrdinal(t *testing.T) {
	const chatID int64 = -1009000000805
	ctx := context.Background()
	db, err := Open(ctx, testSQLiteConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := ensureChat(ctx, db, chatID); err != nil {
		t.Fatal(err)
	}
	store := NewRuleStore(db)
	current := rules.Record{
		ID: "complete-rule", ChatID: chatID, Collection: "challenge", Ordinal: 0, Enabled: true,
		Definition: json.RawMessage(`{"kind":"contains","value":"answer"}`),
	}
	if _, _, err := store.ReplaceRules(ctx, chatID, current.Collection, nil, []rules.Record{current}); err != nil {
		t.Fatalf("seed complete rule: %v", err)
	}
	next := current
	next.Enabled = false

	for _, test := range []struct {
		name       string
		invalidate func(*rules.Record)
	}{
		{name: "an empty ID", invalidate: func(record *rules.Record) { record.ID = "" }},
		{name: "an empty collection", invalidate: func(record *rules.Record) { record.Collection = "" }},
		{name: "a negative ordinal", invalidate: func(record *rules.Record) { record.Ordinal = -1 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			expected := current
			candidate := next
			test.invalidate(&expected)
			test.invalidate(&candidate)

			_, changed, err := store.UpdateRule(ctx, chatID, expected, candidate)
			if changed || !errors.Is(err, rules.ErrRuleInvalid) {
				t.Fatalf("a rule with %s = changed %t, err %v; want invalid-rule refusal before a conditional update reports a conflict", test.name, changed, err)
			}
		})
	}

	updated, changed, err := store.UpdateRule(ctx, chatID, current, next)
	if err != nil || !changed || updated.Enabled {
		t.Fatalf("complete rule update = record %+v, changed %t, err %v; want a successful disabled update", updated, changed, err)
	}
}
