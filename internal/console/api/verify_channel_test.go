package api

import (
	"context"
	"html"
	"net/http"
	"strings"
	"testing"

	"github.com/Zakkaus/vestibule/internal/settings"
)

func TestWebProofChecksRequiredChannelBeforeApproval(t *testing.T) {
	f := newAPIWebFixture(t, settings.ModePoW, "en")
	channelID := apiWebChatID - 1
	channelDisplay := "@test_required_channel"
	channelInvite := "https://t.me/+test_private_invite"
	view, _ := f.settings.Settings(apiWebChatID)
	if _, err := f.settings.Update(apiWebChatID, view.Revision(), settings.GroupOverrides{RequiredChannelID: &channelID, ChannelDisplay: &channelDisplay, ChannelInviteURL: &channelInvite}); err != nil {
		t.Fatal(err)
	}
	f.request(http.MethodPost, f.raw, "nonce=invalid")
	if f.gateway.channelReads.Load() != 0 {
		t.Fatal("invalid proof performed a channel check")
	}
	proof := "nonce=" + apiWebNonce(t, f.token)
	response := f.request(http.MethodPost, f.raw, proof)
	if !strings.Contains(html.UnescapeString(response.Body.String()), `href="`+channelInvite+`"`) || !strings.Contains(response.Body.String(), channelDisplay) {
		t.Fatal("channel-required page omitted the configured private-channel invitation")
	}
	record, _, valid, err := f.service.ResolveWebToken(context.Background(), f.raw)
	if err != nil || !valid || record.Tries != 1 || f.gateway.approvals.Load() != 0 || f.gateway.channelReads.Load() != 1 {
		t.Fatalf("channel requirement settled or charged another attempt: tries=%d valid=%v err=%v", record.Tries, valid, err)
	}
	f.gateway.channelMember.Store(true)
	f.request(http.MethodPost, f.raw, proof)
	if f.gateway.approvals.Load() != 1 || f.gateway.channelReads.Load() != 2 {
		t.Fatal("channel membership did not admit the valid proof")
	}
}
