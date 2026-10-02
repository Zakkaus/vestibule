package verification

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/settings"
)

type fallbackCommitStore struct {
	testVerificationStore
	WebStore
	committed chan struct{}
	resume    chan struct{}
	epoch     atomic.Uint64
	updated   chan struct{}
}

func (s *fallbackCommitStore) FallbackWeb(_ context.Context, _ PendingRef, next PendingRecord, _ *WebClaim) (bool, error) {
	s.epoch.Store(next.Epoch)
	close(s.committed)
	<-s.resume
	return true, nil
}

func (s *fallbackCommitStore) UpdatePending(_ string, ref PendingRef, _ PendingRecord) (bool, error) {
	defer close(s.updated)
	return ref.Epoch == s.epoch.Load(), nil
}

func TestFallbackCommitCannotLosePendingToConcurrentChannelReading(t *testing.T) {
	const gid, uid = int64(-1009000000891), int64(891)
	v := newTestService(&settings.Config{GroupIDs: []int64{gid}})
	p := &pending{mode: settings.ModeCaptcha, nonce: "race", deadline: time.Now().Add(time.Hour)}
	key := pkey{gid, uid}
	v.pend[key] = p
	store := &fallbackCommitStore{committed: make(chan struct{}), resume: make(chan struct{}), updated: make(chan struct{})}
	v.statePath, p.persistedPath, v.stateStore = "durable", "durable", store
	id := ChallengeID(pendingRecord(key, p).Ref())
	result := make(chan bool, 1)
	go func() {
		changed, _ := v.FallbackWeb(context.Background(), id)
		result <- changed
	}()
	<-store.committed
	channelRead := make(chan struct{})
	go func() {
		v.markChannelReadable(gid, uid, true)
		close(channelRead)
	}()
	select {
	case <-store.updated:
	case <-time.After(time.Second):
	}
	close(store.resume)
	changed := <-result
	<-channelRead
	if !changed || v.pend[key] != p || p.done || settings.IsWebMode(p.mode) || !p.fallbackPending || p.epoch != store.epoch.Load() {
		t.Fatalf("committed fallback lost its pending question: changed=%v pending=%+v", changed, p)
	}
}
