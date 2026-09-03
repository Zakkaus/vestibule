package verification

import (
	"errors"
	"strings"
	"testing"
)

type recordedStoreWrites struct {
	successes int
	failures  int
}

func (r *recordedStoreWrites) RecordDatabaseWrite(success bool) {
	if success {
		r.successes++
		return
	}
	r.failures++
}

// SQLite returns a busy error when another writer holds the database, and settling a
// challenge writes the transition and its outbox row together. Retrying is what turns that
// transient collision into a settled challenge; giving up on the first one abandons the
// settlement and leaves the applicant in whatever state the failed transition left them.
// The retry count was not held anywhere: every test that touches it reads the same constant
// it is testing, so lowering the policy to a single attempt left the suite green.
func TestADurableStateWriteSurvivesTwoTransientFailures(t *testing.T) {
	transient := errors.New("database is locked")
	calls := 0
	observer := &recordedStoreWrites{}

	err := retryStoreWrite(observer, func() error {
		calls++
		if calls <= 2 {
			return transient
		}
		return nil
	})
	if err != nil {
		t.Fatalf("state write gave up after %d attempt(s): %v; a settlement is abandoned "+
			"and the applicant is left mid-transition", calls, err)
	}
	if calls != 3 {
		t.Fatalf("state write took %d attempt(s) to get through, want 3", calls)
	}
	if observer.successes != 1 || observer.failures != 0 {
		t.Fatalf("write observations = %+v, want one success", observer)
	}

	calls = 0
	changed, err := retryStoreChange(func() (bool, error) {
		calls++
		if calls <= 2 {
			return false, transient
		}
		return true, nil
	})
	if err != nil || !changed {
		t.Fatalf("conditional state write gave up after %d attempt(s) = %t, %v", calls, changed, err)
	}
	if calls != 3 {
		t.Fatalf("conditional state write took %d attempt(s) to get through, want 3", calls)
	}
}

// The other side of the same policy: three failures is where it stops, the error says how
// many attempts were made, and the outcome is recorded once rather than once per attempt.
func TestADurableStateWriteStopsAfterThreeFailures(t *testing.T) {
	transient := errors.New("database is locked")
	calls := 0
	observer := &recordedStoreWrites{}

	err := retryStoreWrite(observer, func() error {
		calls++
		return transient
	})
	if err == nil {
		t.Fatal("a state write that never succeeds reported success")
	}
	if calls != 3 {
		t.Fatalf("failing state write was attempted %d time(s), want 3", calls)
	}
	if !errors.Is(err, transient) || !strings.Contains(err.Error(), "3 attempts") {
		t.Fatalf("give-up error = %v, want the cause and the number of attempts", err)
	}
	if observer.successes != 0 || observer.failures != 1 {
		t.Fatalf("write observations = %+v, want one failure", observer)
	}

	calls = 0
	if _, err = retryStoreChange(func() (bool, error) {
		calls++
		return false, transient
	}); err == nil || calls != 3 {
		t.Fatalf("failing conditional state write was attempted %d time(s) = %v, want 3", calls, err)
	}
}
