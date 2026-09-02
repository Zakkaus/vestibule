package verification

import (
	"context"
	"errors"
	"testing"

	"github.com/Zakkaus/vestibule/internal/settings"
)

// A deactivated applicant produced ten declines a minute apart, each one impossible, ending in a
// warning asking an administrator to settle a request nobody can settle.
func TestGiveUpSettlingOnAnImpossibleFailure(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "deactivated applicant", err: testGatewayError(errors.New(`api: 403 "Forbidden: user is deactivated"`)), want: true},
		{name: "bot removed from the group", err: testGatewayError(errors.New("Forbidden: bot is not a member of the supergroup chat")), want: true},
		{name: "transient network failure", err: errors.New("unexpected EOF"), want: false},
		{name: "missing rights are repairable", err: errors.New(`api: 400 "Bad Request: not enough rights"`), want: false},
	}
	for _, tt := range tests {
		if got := giveUpSettling(tt.err); got != tt.want {
			t.Errorf("%s: giveUpSettling = %v, want %v", tt.name, got, tt.want)
		}
	}
}

// The operator notice for a settlement failure is suppressed for one error only: an applicant
// whose Telegram account no longer exists. That suppression is a decision, not a side effect, so
// both directions are pinned here — a deactivated applicant stays silent, and every other failure
// still reaches an operator. Production sees this path about twice a week on the live group.
func TestSettlementAlertStaysSilentOnlyForADeactivatedApplicant(t *testing.T) {
	for _, tc := range []struct {
		name  string
		err   error
		quiet bool
	}{
		{"deactivated applicant", testGatewayError(errors.New(`api: 403 "Forbidden: user is deactivated"`)), true},
		{"any other failure", testGatewayError(errors.New(`api: 400 "Bad Request: chat not found"`)), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := newTestService(&settings.Config{})
			fb := &fakeVerifyBot{}
			v.settlementAlert(context.Background(), fb, -100, tc.err, "settlement failed")
			if tc.quiet && fb.sends != 0 {
				t.Errorf("a deactivated applicant produced %d operator notices, want none", fb.sends)
			}
			if !tc.quiet && fb.sends != 1 {
				t.Errorf("an actionable failure produced %d operator notices, want one", fb.sends)
			}
		})
	}
}
