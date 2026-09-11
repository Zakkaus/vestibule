package verification

import "testing"

func TestDailyPendingCountExcludesSettledChallengesAcrossGroups(t *testing.T) {
	first := &pending{}
	service := &Service{pend: map[pkey]*pending{
		{gid: -1009000000801, uid: 1}: first,
		{gid: -1009000000802, uid: 2}: {},
		{gid: -1009000000801, uid: 3}: {done: true},
		{gid: -1009000000802, uid: 4}: nil,
	}}
	if got := service.PendingCount(); got != 2 {
		t.Fatalf("pending total = %d, want two live challenges across groups", got)
	}
	first.done = true
	if got := service.PendingCount(); got != 1 {
		t.Fatalf("pending total after settlement = %d, want one", got)
	}
}
