package verification

import (
	"context"
	"errors"
	"testing"
	"time"
)

type rejectionContextKey struct{}

type rejectionStoreProbe struct {
	testVerificationStore
	gotContext context.Context
	gotSince   int64
	counts     []RejectionReasonCount
	err        error
}

func (s *rejectionStoreProbe) LoadRecentRejectionReasons(
	ctx context.Context,
	since int64,
) ([]RejectionReasonCount, error) {
	s.gotContext = ctx
	s.gotSince = since
	return s.counts, s.err
}

func recentRejectionsWithoutPanic(
	service *Service,
	ctx context.Context,
	since time.Time,
) (counts []RejectionReasonCount, panicValue any, err error) {
	defer func() { panicValue = recover() }()
	counts, err = service.RecentRejections(ctx, since)
	return counts, nil, err
}

func TestRecentRejectionsRefusesUnavailableStoreAndAcceptsCapableStore(t *testing.T) {
	since := time.Unix(1_800_000_000, 0)
	unavailable := &Service{stateStore: testVerificationStore{}}
	counts, panicValue, err := recentRejectionsWithoutPanic(unavailable, context.Background(), since)
	if panicValue != nil {
		t.Fatalf("a store without cutover observations panicked instead of reporting unavailability: %v", panicValue)
	}
	if counts != nil || !errors.Is(err, ErrRollbackRejectionsUnavailable) {
		t.Fatalf("unavailable store returned counts=%v error=%v, want no counts and ErrRollbackRejectionsUnavailable", counts, err)
	}

	capableStore := &rejectionStoreProbe{counts: []RejectionReasonCount{{Count: 2}}}
	capable := &Service{stateStore: capableStore}
	counts, panicValue, err = recentRejectionsWithoutPanic(capable, context.Background(), since)
	if panicValue != nil || err != nil || len(counts) != 1 || counts[0].Count != 2 {
		t.Fatalf("capable store returned counts=%v error=%v panic=%v, want its available observations", counts, err, panicValue)
	}
}

func TestRecentRejectionsPreservesRequestBoundaryResultAndFailure(t *testing.T) {
	ctx := context.WithValue(context.Background(), rejectionContextKey{}, "request")
	since := time.Date(2026, time.September, 3, 23, 45, 6, 0, time.FixedZone("cutover", 9*60*60))
	reason := "wrong_answer"
	want := []RejectionReasonCount{{Reason: &reason, Count: 3}}
	store := &rejectionStoreProbe{counts: want}
	service := &Service{stateStore: store}

	got, err := service.RecentRejections(ctx, since)
	if err != nil || len(got) != 1 || got[0].Reason != want[0].Reason || got[0].Count != want[0].Count {
		t.Fatalf("recent rejection result=%v error=%v, want the store result", got, err)
	}
	if store.gotContext.Value(rejectionContextKey{}) != "request" {
		t.Fatal("recent rejection lookup discarded the request context, so cancellation cannot reach storage")
	}
	if store.gotSince != since.Unix() {
		t.Fatalf("recent rejection boundary=%d, want exact instant %d", store.gotSince, since.Unix())
	}

	failure := errors.New("rejection query unavailable")
	store.counts, store.err = nil, failure
	got, err = service.RecentRejections(ctx, since)
	if got != nil || !errors.Is(err, failure) {
		t.Fatalf("failed rejection lookup returned counts=%v error=%v, want the storage failure", got, err)
	}
}
