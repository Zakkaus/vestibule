package verification

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/settings"
)

func TestFullPendingQueueReusesTheApplicantsExistingSlot(t *testing.T) {
	const firstGroup int64 = -1009000003001
	v := newTestService(&settings.Config{
		GroupIDs:       []int64{firstGroup},
		TimeoutSeconds: 3600,
	})
	v.statePath = filepath.Join(t.TempDir(), "pending.json")

	var current *pending
	deadline := time.Now().Add(time.Hour)
	for group := range pendingGlobalCap / pendingPerGroupCap {
		gid := firstGroup - int64(group)
		for member := range pendingPerGroupCap {
			p := &pending{nonce: "occupied", deadline: deadline}
			v.pend[pkey{gid, int64(member + 1)}] = p
			if group == 0 && member == 0 {
				current = p
			}
		}
	}

	extra := &pending{nonce: "extra"}
	if _, status, err := v.startPending(newFakeVerifyBot(), firstGroup-10, 1, extra); err != nil {
		t.Fatal(err)
	} else if status != pendingBlockedCapacity {
		t.Fatalf("new applicant at full capacity got status %v, want blocked: the queue bound must still refuse a new slot", status)
	}

	replacement := &pending{nonce: "replacement"}
	if _, status, err := v.startPending(newFakeVerifyBot(), firstGroup, 1, replacement); err != nil {
		t.Fatal(err)
	} else if status != pendingStarted {
		t.Fatalf("current applicant replacement at full capacity got status %v, want started: their existing slot must not trap them in manual review", status)
	}
	if got := len(v.pend); got != pendingGlobalCap {
		t.Fatalf("pending queue grew to %d during replacement, want %d", got, pendingGlobalCap)
	}
	if v.pend[pkey{firstGroup, 1}] != replacement || !current.done {
		t.Fatal("full-queue replacement did not retire the old challenge and keep the applicant's slot")
	}
}

func TestPendingCapacityAlertsResumeAtTheCooldownBoundary(t *testing.T) {
	const (
		firstGroup  int64 = -1009000003101
		secondGroup int64 = -1009000003102
	)
	v := newTestService(&settings.Config{GroupIDs: []int64{firstGroup, secondGroup}})
	start := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	moment := start
	v.timeNow = func() time.Time { return moment }
	bot := newFakeVerifyBot()

	v.alertPendingCap(context.Background(), bot, firstGroup, gateRequest)
	if bot.sends != 1 {
		t.Fatalf("first capacity event sent %d alerts, want 1", bot.sends)
	}
	moment = start.Add(pendingCapAlertCooldown - time.Nanosecond)
	v.alertPendingCap(context.Background(), bot, secondGroup, gateRequest)
	if bot.sends != 1 {
		t.Fatalf("capacity events inside the cooldown sent %d alerts, want 1: a flood must not spam operators", bot.sends)
	}
	moment = start.Add(pendingCapAlertCooldown)
	v.alertPendingCap(context.Background(), bot, secondGroup, gateRequest)
	if bot.sends != 2 {
		t.Fatalf("capacity alerts at the cooldown boundary total %d, want 2: operators must hear about a continuing capacity problem", bot.sends)
	}
}

func TestChannelAccessAlertsResumeAtTheCooldownBoundary(t *testing.T) {
	const (
		groupID   int64 = -1009000003201
		channelID int64 = -1009000003202
	)
	v := newTestService(&settings.Config{GroupIDs: []int64{groupID}, AdminLogChatID: groupID})
	start := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	moment := start
	v.timeNow = func() time.Time { return moment }
	bot := newFakeVerifyBot()

	v.channelAccessAlert(context.Background(), bot, groupID, i18n.LangEN, channelID, false)
	if bot.sends != 1 {
		t.Fatalf("first channel access failure sent %d alerts, want 1", bot.sends)
	}
	moment = start.Add(channelAccessAlertCooldown - time.Nanosecond)
	v.channelAccessAlert(context.Background(), bot, groupID, i18n.LangEN, channelID, false)
	if bot.sends != 1 {
		t.Fatalf("channel failures inside the cooldown sent %d alerts, want 1: one unreadable channel must not spam operators", bot.sends)
	}
	moment = start.Add(channelAccessAlertCooldown)
	v.channelAccessAlert(context.Background(), bot, groupID, i18n.LangEN, channelID, false)
	if bot.sends != 2 {
		t.Fatalf("channel alerts at the cooldown boundary total %d, want 2: an unreadable channel must become reportable again", bot.sends)
	}
}

func TestChannelAccessAlertForgetsEntriesAtTheCooldownBoundary(t *testing.T) {
	const (
		groupID        int64 = -1009000003301
		expiredChannel int64 = -1009000003302
		activeChannel  int64 = -1009000003303
		currentChannel int64 = -1009000003304
	)
	v := newTestService(&settings.Config{GroupIDs: []int64{groupID}, AdminLogChatID: groupID})
	start := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	moment := start.Add(channelAccessAlertCooldown)
	v.timeNow = func() time.Time { return moment }
	v.chanAlert[expiredChannel] = start
	v.chanAlert[activeChannel] = start.Add(time.Nanosecond)

	v.channelAccessAlert(context.Background(), newFakeVerifyBot(), groupID, i18n.LangEN, currentChannel, true)

	if _, exists := v.chanAlert[expiredChannel]; exists {
		t.Fatal("channel alert state at the cooldown boundary was retained: expired channels can grow the throttle map without bound")
	}
	if _, exists := v.chanAlert[activeChannel]; !exists {
		t.Fatal("channel alert pruning discarded a cooldown that still had time remaining")
	}
	if got := len(v.chanAlert); got != 2 {
		t.Fatalf("channel alert throttle retained %d entries, want the active and current channels", got)
	}
}
