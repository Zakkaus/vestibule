package app

import (
	"errors"
	"net"
	"testing"

	"github.com/Zakkaus/vestibule/internal/console/api"
	"github.com/mymmrac/telego/telegoapi"
)

// The setup page can only name the cause the reader can act on if this
// classification survives. Collapsing it to one answer tells everyone whose
// machine cannot reach Telegram to re-paste a token that was never at fault.
func TestClaimFailureSeparatesARefusedTokenFromAnUnreachableTelegram(t *testing.T) {
	refused := claimFailureFor(&telegoapi.Error{ErrorCode: 401, Description: "Unauthorized"})
	if !errors.Is(refused, api.ErrSetupTokenRejected) {
		t.Fatalf("a 401 from Telegram classified as %v, want a refused token", refused)
	}

	for name, cause := range map[string]error{
		"dial failure":   &net.OpError{Op: "dial", Err: errors.New("connection refused")},
		"plain timeout":  errors.New("context deadline exceeded"),
		"server trouble": &telegoapi.Error{ErrorCode: 500, Description: "Internal Server Error"},
	} {
		if got := claimFailureFor(cause); !errors.Is(got, api.ErrSetupTelegramUnreachable) {
			t.Fatalf("%s classified as %v, want an unreachable Telegram", name, got)
		}
	}

	if errors.Is(claimFailureFor(&telegoapi.Error{ErrorCode: 401}), api.ErrSetupTelegramUnreachable) {
		t.Fatal("a refused token also reads as unreachable, so the page cannot tell the two apart")
	}
}
