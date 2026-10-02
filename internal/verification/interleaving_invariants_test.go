package verification_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	v "github.com/Zakkaus/vestibule/internal/verification"
)

func chaosDecode(t *testing.T, encoded string, target any) {
	t.Helper()
	chaosRequire(t, json.Unmarshal([]byte(encoded), target))
}

func (f *chaosFixture) failureCounts() map[chaosPair]v.FailureRecord {
	records, err := f.store.LoadFailures("")
	chaosRequire(f.t, err)
	counts := make(map[chaosPair]v.FailureRecord)
	for _, record := range records {
		counts[chaosPair{record.GroupID, record.UserID}] = record
	}
	return counts
}

func (f *chaosFixture) check(before map[chaosPair]v.FailureRecord) []chaosFinding {
	findings := append([]chaosFinding(nil), f.findings...)
	f.findings = nil
	pending := f.pending()
	live := make(map[string]v.PendingRecord)
	pairs := make(map[chaosPair]map[string]bool)
	questions := make(map[[2]int64]bool)
	for _, record := range pending {
		id := chaosID(record)
		live[id] = record
		chaosAddLive(pairs, record)
		questions[[2]int64{record.GroupID, int64(record.GroupMsgID)}] = true
	}
	memory := f.service.InterleavingSnapshot()
	for _, record := range memory.Live {
		chaosAddLive(pairs, record)
		if _, ok := live[chaosID(record)]; !ok {
			findings = append(findings, chaosFinding{"settled-and-pending", "in-memory live challenge is not durably pending"})
		}
	}
	for _, ids := range pairs {
		if len(ids) > 1 {
			findings = append(findings, chaosFinding{"one-live-challenge", "multiple nonces live for one group/user"})
		}
	}
	findings = append(findings, f.checkHolds(pending)...)
	findings = append(findings, f.checkPasses(memory)...)
	findings = append(findings, f.checkStrikes(before)...)
	var dualState int
	chaosRequire(f.t, f.db.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM challenge WHERE state='pending' AND settled_at IS NOT NULL").Scan(&dualState))
	if dualState > 0 {
		findings = append(findings, chaosFinding{"settled-and-pending", "durably pending challenge has a settlement timestamp"})
	}
	if f.actionCount(false) == 0 {
		for question := range f.bot.questions {
			if !questions[question] {
				key := f.classify("orphan-group-question", fmt.Sprint(question))
				findings = append(findings, chaosFinding{key, "posted group question remains after actions drained"})
			}
		}
	}
	return findings
}

func (f *chaosFixture) checkStrikes(before map[chaosPair]v.FailureRecord) []chaosFinding {
	var findings []chaosFinding
	after := f.failureCounts()
	for pair, cause := range f.bot.noFaultCalls {
		previous, current := before[pair], after[pair]
		if current.Count > previous.Count || (current.Count > 0 && current.Last != previous.Last) {
			findings = append(findings, chaosFinding{"no-fault-strike/" + cause, "strike increased on " + cause + " settlement"})
		}
	}
	return findings
}

func chaosAddLive(pairs map[chaosPair]map[string]bool, record v.PendingRecord) {
	pair := chaosPair{record.GroupID, record.UserID}
	if pairs[pair] == nil {
		pairs[pair] = make(map[string]bool)
	}
	pairs[pair][chaosID(record)] = true
}

func (f *chaosFixture) checkHolds(pending []v.PendingRecord) []chaosFinding {
	covered := make(map[chaosPair]bool)
	for _, record := range pending {
		covered[chaosPair{record.GroupID, record.UserID}] = true
	}
	rows, err := f.db.Query(context.Background(), `SELECT a.payload,c.id,c.state,a.kind FROM pending_action a
		JOIN challenge c ON c.id=a.challenge_id WHERE a.state='pending'
		AND a.kind IN ('settle_approve','settle_decline','settle_ban','release_verification_hold')`)
	chaosRequire(f.t, err)
	defer rows.Close()
	for rows.Next() {
		var encoded, id, state, kind string
		chaosRequire(f.t, rows.Scan(&encoded, &id, &state, &kind))
		var payload struct {
			Record v.PendingRecord `json:"record"`
		}
		chaosDecode(f.t, encoded, &payload)
		if payload.Record.Gate == "mute" && (kind == "release_verification_hold" || (state != "superseded" && !f.canceled[id])) {
			covered[chaosPair{payload.Record.GroupID, payload.Record.UserID}] = true
		}
	}
	chaosRequire(f.t, rows.Err())
	var findings []chaosFinding
	for pair, member := range f.bot.members {
		if member.present && member.heldUntil > f.now.Unix() && !covered[pair] {
			key := f.classify("held-without-release", fmt.Sprint(pair))
			findings = append(findings, chaosFinding{key, "verification hold has no pending challenge or queued release/removal"})
		}
	}
	return findings
}

func (f *chaosFixture) checkPasses(memory v.InterleavingMemory) []chaosFinding {
	var findings []chaosFinding
	for _, pass := range memory.Passed {
		pair := chaosPair{pass.GroupID, pass.UserID}
		at, admitted := f.bot.admitted[pair]
		if !admitted || f.now.Sub(at) > 5*time.Minute {
			findings = append(findings, chaosFinding{"recent-pass-window", "suppression active without admission in the last five minutes"})
		}
	}
	return findings
}

func (f *chaosFixture) run(steps []string) map[string]int {
	found := make(map[string]int)
	for index, step := range steps {
		f.currentStep, f.lastFault = step, ""
		before := f.failureCounts()
		f.bot.noFaultCalls = make(map[chaosPair]string)
		f.step(step)
		for _, finding := range f.check(before) {
			if _, exists := found[finding.invariant]; !exists {
				found[finding.invariant] = index
				f.t.Logf("invariant=%s detail=%s step=%s", finding.invariant, finding.detail, step)
			}
		}
	}
	return found
}

func (f *chaosFixture) classify(invariant, identity string) string {
	witness := invariant + identity
	if key := f.witnesses[witness]; key != "" {
		return key
	}
	cause := strings.Split(f.currentStep, "/")[0]
	if f.lastFault != "" {
		cause = f.lastFault
	}
	key := invariant + "/" + cause
	f.witnesses[witness] = key
	return key
}

func runChaos(t *testing.T, steps []string) map[string]int {
	t.Helper()
	return runChaosObserved(t, steps, nil)
}

func runChaosObserved(t *testing.T, steps []string, coverage map[string]int) map[string]int {
	t.Helper()
	f := newChaosFixture(t)
	defer func() { chaosRequire(t, f.db.Close()) }()
	findings := f.run(steps)
	if coverage != nil {
		for fault, count := range f.bot.calls {
			coverage[fault] += count
		}
	}
	return findings
}

func chaosReplay(steps []string) string {
	encoded, err := json.Marshal(steps)
	if err != nil {
		panic(fmt.Sprintf("encode replay: %v", err))
	}
	return string(encoded)
}
