package database

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Zakkaus/vestibule/internal/rules"
)

// Replacing one group's rules deletes that group's rows for that collection and inserts the
// new ones. The delete is scoped twice, by chat and by collection, and only the collection
// half was held: dropping the chat_id predicate made one group's edit delete every group's
// rules in that collection, with every test in the repository still passing. Phase eight's
// whole point is that a group's configuration is its own.
func TestReplaceRulesTouchesOnlyItsOwnGroup(t *testing.T) {
	const (
		edited     int64 = -1009000000801
		untouched  int64 = -1009000000802
		collection       = "challenge"
	)
	ctx := context.Background()
	db, err := Open(ctx, testSQLiteConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for _, chatID := range []int64{edited, untouched} {
		if err := ensureChat(ctx, db, chatID); err != nil {
			t.Fatal(err)
		}
	}
	store := NewRuleStore(db)

	seed := func(chatID int64, id string) []rules.Record {
		record := rules.Record{
			ID: id, ChatID: chatID, Collection: collection, Ordinal: 0, Enabled: true,
			Definition: json.RawMessage(`{"kind":"contains","value":"seed"}`),
		}
		stored, _, err := store.ReplaceRules(ctx, chatID, collection, nil, []rules.Record{record})
		if err != nil {
			t.Fatalf("seeding %d: %v", chatID, err)
		}
		return stored
	}
	// The neighbour is seeded first so that the assertion below is what fails when the scope
	// is dropped. Seeded second, its own write would already have deleted the edited group's
	// rows and the replace would fail as a conflict instead -- true, but it names the wrong
	// thing.
	neighbour := seed(untouched, "rule-untouched")
	before := seed(edited, "rule-edited")

	replacement := []rules.Record{{
		ID: "rule-replacement", ChatID: edited, Collection: collection, Ordinal: 0, Enabled: true,
		Definition: json.RawMessage(`{"kind":"contains","value":"replacement"}`),
	}}
	if _, _, err := store.ReplaceRules(ctx, edited, collection, before, replacement); err != nil {
		t.Fatal(err)
	}

	after, err := store.ListRules(ctx, untouched, collection)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(neighbour) {
		t.Fatalf("the untouched group holds %d rules after another group was edited, want %d; "+
			"one group's edit reached across the instance", len(after), len(neighbour))
	}
	if after[0].ID != neighbour[0].ID {
		t.Errorf("the untouched group's rule is now %q, want %q", after[0].ID, neighbour[0].ID)
	}

	edits, err := store.ListRules(ctx, edited, collection)
	if err != nil {
		t.Fatal(err)
	}
	if len(edits) != 1 || edits[0].ID != "rule-replacement" {
		t.Errorf("the edited group holds %+v, want only the replacement", edits)
	}
}
