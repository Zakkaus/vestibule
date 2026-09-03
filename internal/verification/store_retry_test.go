package verification

import (
	"errors"
	"strings"
	"testing"
	"time"
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

func TestStoreWriteStopsAfterItsFirstSuccessfulAttempt(t *testing.T) {
	for _, successAt := range []int{1, 2} {
		calls := 0
		observer := &recordedStoreWrites{}
		err := retryStoreWrite(observer, func() error {
			calls++
			if calls < successAt {
				return errors.New("temporary write failure")
			}
			return nil
		})
		if err != nil {
			t.Fatalf("success on attempt %d returned error: %v", successAt, err)
		}
		if calls != successAt {
			t.Fatalf("success on attempt %d caused %d writes; a successful write must not be repeated",
				successAt, calls)
		}
		if observer.successes != 1 || observer.failures != 0 {
			t.Fatalf("success on attempt %d recorded %+v, want one successful logical write",
				successAt, observer)
		}
	}
}

func TestStoreRetriesPreserveTheFinalFailure(t *testing.T) {
	failures := []error{
		errors.New("first storage failure"),
		errors.New("second storage failure"),
		errors.New("final storage failure"),
	}
	t.Run("write", func(t *testing.T) {
		calls := 0
		err := retryStoreWrite(nil, func() error {
			failure := failures[calls]
			calls++
			return failure
		})
		if !errors.Is(err, failures[2]) || !strings.Contains(err.Error(), "3 attempts") {
			t.Fatalf("write retry error %v, want final storage failure %v after three attempts", err, failures[2])
		}
	})
	t.Run("conditional write", func(t *testing.T) {
		calls := 0
		_, err := retryStoreChange(func() (bool, error) {
			failure := failures[calls]
			calls++
			return false, failure
		})
		if !errors.Is(err, failures[2]) || !strings.Contains(err.Error(), "3 attempts") {
			t.Fatalf("conditional retry error %v, want final storage failure %v after three attempts",
				err, failures[2])
		}
	})
}

func TestStoreChangeReturnsSuccessfulUnchangedResultWithoutRetry(t *testing.T) {
	calls := 0
	changed, err := retryStoreChange(func() (bool, error) {
		calls++
		if calls == 1 {
			return false, errors.New("temporary conditional write failure")
		}
		return false, nil
	})
	if err != nil || changed {
		t.Fatalf("successful unchanged write returned changed=%v error=%v", changed, err)
	}
	if calls != 2 {
		t.Fatalf("successful unchanged write caused %d calls, want no retry after attempt 2", calls)
	}
}

func TestStoreRetryBackoffWaitsBetweenAttemptsButNotAfterExhaustion(t *testing.T) {
	for _, test := range []struct {
		name string
		run  func() []time.Time
	}{
		{name: "write", run: failedWriteAttemptTimes},
		{name: "change", run: failedChangeAttemptTimes},
	} {
		first, second := minimumRetryIntervals(t, test.run)
		if first < 10*time.Millisecond {
			t.Errorf("%s first retry waited %s, want at least 10ms to avoid hammering failed storage",
				test.name, first)
		}
		if second < 20*time.Millisecond {
			t.Errorf("%s second retry waited %s, want at least 20ms linear backoff", test.name, second)
		}
	}

	started := time.Now()
	for range 10 {
		pauseBeforeStoreRetry(3)
	}
	if elapsed := time.Since(started); elapsed >= 200*time.Millisecond {
		t.Fatalf("terminal attempts added %s of backoff although no retry follows exhaustion", elapsed)
	}
}

func minimumRetryIntervals(t *testing.T, run func() []time.Time) (time.Duration, time.Duration) {
	t.Helper()
	first, second := time.Duration(1<<63-1), time.Duration(1<<63-1)
	for range 3 {
		attempts := run()
		if len(attempts) != 3 {
			t.Fatalf("failed storage write ran %d attempts while measuring backoff, want 3", len(attempts))
		}
		if interval := attempts[1].Sub(attempts[0]); interval < first {
			first = interval
		}
		if interval := attempts[2].Sub(attempts[1]); interval < second {
			second = interval
		}
	}
	return first, second
}

func failedWriteAttemptTimes() []time.Time {
	attempts := make([]time.Time, 0, 3)
	_ = retryStoreWrite(nil, func() error {
		attempts = append(attempts, time.Now())
		return errors.New("write unavailable")
	})
	return attempts
}

func failedChangeAttemptTimes() []time.Time {
	attempts := make([]time.Time, 0, 3)
	_, _ = retryStoreChange(func() (bool, error) {
		attempts = append(attempts, time.Now())
		return false, errors.New("change unavailable")
	})
	return attempts
}
