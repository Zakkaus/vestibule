package app

import (
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/verification"
)

func TestRetentionAlertWaitsUntilTelegramCanNoLongerReplayUpdates(t *testing.T) {
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	lastOnline := now.Add(-telegramUpdateRetention)
	alerts := 0
	observer := retentionOutageObserver{
		loadHeartbeat: func() (verification.HeartbeatRecord, error) {
			return verification.HeartbeatRecord{LastOnline: lastOnline.Unix()}, nil
		},
		alert: func(time.Duration) { alerts++ },
	}

	observer.observe(now)
	if alerts != 0 {
		t.Fatalf("exactly one retention window produced %d alerts, want 0: Telegram can still replay those updates", alerts)
	}
	lastOnline = now.Add(-telegramUpdateRetention - time.Second)
	observer.observe(now)
	if alerts != 1 {
		t.Fatalf("an outage beyond the retention window produced %d alerts, want 1: administrators must be warned when updates may be lost", alerts)
	}
}

func TestHeartbeatOutageRejectsTimestampsThatCannotDescribeDowntime(t *testing.T) {
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		lastOnline int64
		wantOutage time.Duration
		wantOK     bool
		harm       string
	}{
		{
			name: "missing heartbeat", harm: "a first startup must not raise a false retention alert",
		},
		{
			name: "invalid negative heartbeat", lastOnline: -1,
			harm: "invalid persisted state must not become a decades-long outage",
		},
		{
			name: "future heartbeat", lastOnline: now.Add(time.Second).Unix(),
			harm: "clock skew must not create a negative outage",
		},
		{
			name: "past heartbeat", lastOnline: now.Add(-time.Hour).Unix(),
			wantOutage: time.Hour, wantOK: true,
			harm: "valid outages must still reach retention handling",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outage, ok := heartbeatOutage(tt.lastOnline, now)
			if outage != tt.wantOutage || ok != tt.wantOK {
				t.Fatalf("heartbeat produced outage (%s, %t), want (%s, %t): %s", outage, ok, tt.wantOutage, tt.wantOK, tt.harm)
			}
		})
	}
}
