package settings

import (
	"testing"
	"time"
)

// The claim link is minted at a terminal and consumed in Telegram, which is a
// different device and often a different room. Ten minutes turned "I will do
// that in a moment" into restarting the process to mint another, and there is no
// failure cost to weigh against that: the link is still one use and still expires.
func TestOwnerClaimLifetimeLeavesRoomToReachAPhone(t *testing.T) {
	var unset Config
	if got := unset.OwnerClaimLifetime(); got != time.Hour {
		t.Fatalf("the default owner claim window is %v, want an hour", got)
	}
	for name, seconds := range map[string]int{
		"zero":         0,
		"negative":     -1,
		"over the cap": maxOwnerClaimLifetimeSeconds + 1,
	} {
		config := Config{OwnerClaimLifetimeSeconds: seconds}
		if got := config.OwnerClaimLifetime(); got != time.Hour {
			t.Fatalf("%s fell back to %v, want an hour", name, got)
		}
	}
	configured := Config{OwnerClaimLifetimeSeconds: 300}
	if got := configured.OwnerClaimLifetime(); got != 5*time.Minute {
		t.Fatalf("a configured window became %v, want the five minutes asked for", got)
	}
}
