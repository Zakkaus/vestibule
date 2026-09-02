package database

import (
	"context"
	"testing"

	"github.com/Zakkaus/vestibule/internal/verification"
)

// The console asks for one group's settled history and one group's numbers, and the chat
// predicate in each query is what keeps the answer to that group. Neither was tested:
// dropping either predicate returns every group's rows, and every test in the repository
// still passed. An instance serves many groups, and who was declined or banned in one of
// them is not the neighbouring group's to read.
func TestChallengeAuditAndStatsAnswerOnlyTheGroupAsked(t *testing.T) {
	const asked, neighbour int64 = -1009000000951, -1009000000952
	ctx := context.Background()
	db, err := Open(ctx, testSQLiteConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	state := NewVerificationStore(db)

	// The neighbour is settled first, so a lost predicate shows up as its record appearing in
	// the answer rather than as some later step failing.
	requireAuditTransition(t, state, verification.PendingRecord{
		GroupID: neighbour, UserID: 61, Name: "Neighbour", Nonce: "neighbour", Deadline: 90, Epoch: 1,
	}, verification.ChallengeDeclined, "wrong_answer", 1_000, 9)
	requireAuditTransition(t, state, verification.PendingRecord{
		GroupID: asked, UserID: 62, Name: "Asked", Nonce: "asked", Deadline: 90, Epoch: 1,
	}, verification.ChallengeDeclined, "wrong_answer", 1_010, 9)

	records, err := state.LoadChallengeAudit(ctx, asked)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("audit for %d returned %d records, want 1; another group's settlements are "+
			"not this group's to read", asked, len(records))
	}
	if records[0].Record.GroupID != asked {
		t.Errorf("audit returned a record from group %d", records[0].Record.GroupID)
	}

	counts, err := state.LoadChallengeStats(ctx, asked,
		[]verification.ChallengeStatsBucket{{StartAt: 0, EndAt: 2_000}})
	if err != nil {
		t.Fatal(err)
	}
	var total int64
	for _, count := range counts {
		total += count.Count
	}
	if total != 1 {
		t.Fatalf("stats for %d counted %d settlements, want 1; the neighbour's are counted "+
			"into this group's numbers", asked, total)
	}
}
