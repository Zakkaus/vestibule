package verification_test

import "testing"

type chaosRegression struct {
	name  string
	steps []string
}

func chaosRegressions() []chaosRegression {
	return []chaosRegression{
		{"held-remove", []string{"member", "remove"}},
		{"held-restart-stop", []string{"member", "fail/unrestrict/transient", "right", "restart", "stop"}},
		{"held-cleanup-transient", []string{"member", "fail/unrestrict/transient", "stop"}},
		{"held-cleanup-permanent", []string{"member/1/1", "fail/unrestrict/permanent", "stop/1/0"}},
		{"held-cleanup-request-gone", []string{"fail/unrestrict/request-gone", "member/1/1", "stop/1/1"}},
		{"question-approve-permanent", []string{"join/1/1", "fail/approve/permanent", "right/1/1"}},
		{"question-approve-member-gone", []string{"join/1/1", "fail/approve/member-gone", "right/1/1"}},
		{"question-decline-permanent", []string{"join/1/1", "fail/decline/permanent", "wrong/1/1"}},
		{"question-decline-member-gone", []string{"join/1/1", "fail/decline/member-gone", "wrong/1/1"}},
		{"question-unrestrict-permanent", []string{"member/1/1", "fail/unrestrict/permanent", "right/1/1"}},
		{"question-unrestrict-member-gone", []string{"member/1/1", "fail/unrestrict/member-gone", "right/1/1"}},
		{"question-delete-member-gone", []string{"join/1/1", "fail/delete/member-gone", "timeout/1/1", "due"}},
		{"question-delete-permanent", []string{"join/1/1", "fail/delete/permanent", "right/1/1", "due"}},
		{"question-recovery-delete", []string{"member/1/0", "advance/86401", "fail/delete/request-gone", "restart"}},
		{"question-stop-delete", []string{"join", "fail/delete/transient", "stop"}},
		{"question-delayed-drain", []string{"member/1/0", "console-approve/1/0", "join/0/0", "remove/0/1", "due"}},
		{"question-remove", []string{"join/1/0", "remove/1/1"}},
		{"recent-pass-window", []string{"member/0/1", "fail/unrestrict/transient", "console-approve/0/1", "stop/0/0", "advance/301", "due"}},
		{"settlement-remove", []string{"fail/decline/transient", "join/1/1", "wrong/1/1", "advance/301", "remove/1/0", "due"}},
		{"settlement-stop", []string{"member", "fail/unrestrict/transient", "right", "stop", "advance/6", "due"}},
		{"unheld-pass-window", []string{"fail/restrict/request-gone", "member/1/0", "advance/86401", "right/1/0", "restart"}},
		{"inferred-pass-window", []string{"member/1/1", "right/1/1", "join/1/1", "fail/approve/request-gone", "advance/301", "console-approve/1/1", "restart"}},
	}
}

func TestVerificationLifecycleRegressions(t *testing.T) {
	for _, regression := range chaosRegressions() {
		t.Run(regression.name, func(t *testing.T) {
			for invariant, index := range runChaos(t, regression.steps) {
				t.Errorf("%s at step %d; replay=%s", invariant, index, chaosReplay(regression.steps[:index+1]))
			}
		})
	}
}

func TestRecentPassRestorationKeepsLatestConfirmedAdmission(t *testing.T) {
	f := newChaosFixture(t)
	start := f.now.Unix()
	for _, step := range []string{"join", "right", "due", "advance/10", "join", "right", "due"} {
		f.step(step)
	}
	admittedAt := f.now.Unix()
	for _, step := range []string{"advance/301", "fail/restrict/request-gone", "member", "right", "due"} {
		f.step(step)
	}
	passes, err := f.store.LoadRecentPasses("", start, f.now.Unix())
	chaosRequire(t, err)
	if len(passes) != 1 || passes[0].GroupID != chaosGroups[0] || passes[0].UserID != chaosUsers[0] || passes[0].PassedAt != admittedAt {
		t.Fatalf("restored admissions=%+v; want only the latest confirmed admission at %d", passes, admittedAt)
	}
}
