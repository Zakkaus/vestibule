package verification_test

import (
	"fmt"
	"math/rand/v2"
	"os"
	"slices"
	"strconv"
	"testing"
)

// Replay printed JSON with VERIFICATION_CHAOS_REPLAY='["join",...]'. Long mode
// explores all seeds, reports each invariant, and fails only after completing the search.
func TestVerificationInterleavings(t *testing.T) {
	if replay := os.Getenv("VERIFICATION_CHAOS_REPLAY"); replay != "" {
		var steps []string
		chaosDecode(t, replay, &steps)
		for invariant, index := range runChaos(t, steps) {
			t.Errorf("%s at step %d; replay=%s", invariant, index, chaosReplay(steps[:index+1]))
		}
		return
	}
	if os.Getenv("VERIFICATION_CHAOS_LONG") != "" {
		chaosExplore(t)
		return
	}
	cases := chaosPassingCases()
	if testing.Short() {
		cases = cases[:4]
	}
	for index, steps := range cases {
		t.Run(strconv.Itoa(index), func(t *testing.T) {
			for invariant, at := range runChaos(t, steps) {
				t.Errorf("%s at step %d; replay=%s", invariant, at, chaosReplay(steps[:at+1]))
			}
		})
	}
}

func chaosPassingCases() [][]string {
	return [][]string{
		{"join", "right", "due", "restart", "advance/301", "member", "right", "due"},
		{"join", "wrong", "due", "restart", "advance/2", "join", "console-approve", "due"},
		{"member", "right", "due", "member/1/1", "console-reject/1/1", "due"},
		{"join", "join", "restart", "stop", "due", "join"},
		{"member", "stop", "due", "restart"},
		{"join", "fail/approve/transient", "right", "restart", "advance/6", "due"},
		{"join", "fail/decline/transient", "wrong", "restart", "advance/6", "due"},
		{"member", "fail/unrestrict/transient", "right", "restart", "advance/6", "due"},
		{"join", "fail/delete/transient", "right", "due", "advance/6", "due"},
		{"member", "wrong", "due", "join/1/1", "console-reject/1/1", "due"},
		{"fail/restrict/permanent", "member", "right", "due"},
	}
}

func chaosSeed(seed int64) []string {
	rng := rand.New(rand.NewPCG(uint64(seed), 0))
	starts := [][]string{
		{"join"}, {"member"}, {"join", "advance/31", "restart", "timeout", "due"},
		{"join", "advance/120", "restart", "timeout", "due"},
		{"join", "fail/decline/transient", "wrong", "restart", "stop", "advance/6", "due"},
		{"member", "fail/unrestrict/transient", "right", "restart", "stop", "advance/6", "due"},
		{"member", "remove", "due"}, {"member", "fail/unrestrict/transient", "stop", "due"},
		{"join", "fail/delete/transient", "stop", "due"},
		{"join", "fail/delete/permanent", "right", "due"},
		{"join", "fail/approve/member-gone", "right", "due"},
		{"join", "fail/decline/transient", "wrong", "advance/6", "interrupt/stop", "due"},
		{"join", "fail/approve/transient", "right", "advance/6", "interrupt/remove", "due"},
	}
	steps := append(chaosFaultCase(seed), starts[rng.IntN(len(starts))]...)
	operations := []string{"join", "join", "member", "right", "wrong", "timeout", "restart",
		"stop", "remove", "console-approve", "console-reject", "advance", "fail", "interrupt", "due", "due"}
	calls := []string{"approve", "decline", "restrict", "unrestrict", "delete"}
	faults := []string{"transient", "permanent", "request-gone", "member-gone"}
	seconds := []int{1, 6, 31, 61, 120, 301, 86401}
	for range 36 {
		op := operations[rng.IntN(len(operations))]
		switch op {
		case "advance":
			op = fmt.Sprintf("advance/%d", seconds[rng.IntN(len(seconds))])
		case "fail":
			op = "fail/" + calls[rng.IntN(len(calls))] + "/" + faults[rng.IntN(len(faults))]
		case "interrupt":
			op = "interrupt/" + []string{"stop", "remove"}[rng.IntN(2)]
		case "restart", "due":
		default:
			op = fmt.Sprintf("%s/%d/%d", op, rng.IntN(2), rng.IntN(2))
		}
		steps = append(steps, op)
	}
	return append(steps, "advance/301", "due")
}

// The first twenty seeds consume every operation/failure pair, rather than only arming faults.
func chaosFaultCase(seed int64) []string {
	calls := []string{"approve", "decline", "restrict", "unrestrict", "delete"}
	faults := []string{"transient", "permanent", "request-gone", "member-gone"}
	call, fault := calls[seed%5], faults[(seed/5)%4]
	arm := "fail/" + call + "/" + fault
	switch call {
	case "restrict":
		return []string{arm, "member/1/1", "right/1/1", "advance/6", "due"}
	case "unrestrict":
		return []string{"member/1/1", arm, "right/1/1", "advance/6", "due"}
	case "decline":
		return []string{"join/1/1", arm, "wrong/1/1", "advance/6", "due"}
	default:
		return []string{"join/1/1", arm, "right/1/1", "advance/6", "due"}
	}
}

func chaosExplore(t *testing.T) {
	t.Helper()
	counts := make(map[string]int)
	minimal := make(map[string][]string)
	firstSeed := make(map[string]int64)
	coverage := make(map[string]int)
	for seed := int64(0); seed < 2000; seed++ {
		steps := chaosSeed(seed)
		for invariant, index := range runChaosObserved(t, steps, coverage) {
			counts[invariant]++
			if _, seen := minimal[invariant]; !seen {
				minimal[invariant] = chaosMinimize(t, steps[:index+1], invariant)
				firstSeed[invariant] = seed
			}
		}
	}
	keys := make([]string, 0, len(counts))
	for invariant := range counts {
		keys = append(keys, invariant)
	}
	slices.Sort(keys)
	for _, invariant := range keys {
		t.Errorf("invariant=%s seeds=%d first_seed=%d minimal_replay=%s full_seed_steps=%s",
			invariant, counts[invariant], firstSeed[invariant], chaosReplay(minimal[invariant]),
			chaosReplay(chaosSeed(firstSeed[invariant])))
	}
	t.Logf("completed 2000 seeds; distinct violated invariants=%d; consumed operation/failure pairs=%d/20",
		len(counts), len(coverage))
}

// Repeated single-step deletion reaches a 1-minimal replay, including its fault arm.
func chaosMinimize(t *testing.T, steps []string, invariant string) []string {
	t.Helper()
	steps = slices.Clone(steps)
	for index := 0; index < len(steps); {
		candidate := append(slices.Clone(steps[:index]), steps[index+1:]...)
		if _, fails := runChaos(t, candidate)[invariant]; fails {
			steps, index = candidate, 0
		} else {
			index++
		}
	}
	return steps
}
