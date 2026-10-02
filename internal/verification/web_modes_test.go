package verification

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/settings"
)

func TestWebProofCannotBeBypassedByQuizCallback(t *testing.T) {
	const gid, uid = int64(-1009000000888), int64(888)
	for _, mode := range []string{settings.ModePoW, settings.ModeCaptcha} {
		t.Run(mode, func(t *testing.T) {
			v := newTestService(&settings.Config{GroupIDs: []int64{gid}})
			p := &pending{mode: mode, nonce: "current", correctIdx: -1, deadline: time.Now().Add(time.Hour)}
			v.pend[pkey{gid, uid}] = p
			bot := newFakeVerifyBot()
			update := Update{CallbackQuery: &CallbackQuery{ID: "forged", From: User{ID: uid}, Data: fmt.Sprintf("%s%d:%d:current:-1", AnswerCallbackPrefix, gid, uid)}}
			if err := v.OnAnswer(NewHandlerContext(context.Background(), bot), update); err != nil {
				t.Fatal(err)
			}
			if p.done || bot.approves != 0 || bot.declines != 0 {
				t.Fatal("quiz callback bypassed web proof")
			}
		})
	}
}

func TestNewWebChallengeCapabilityLossUsesQuizWithoutSettingsWrite(t *testing.T) {
	const gid = int64(-1009000000889)
	v := newTestService(&settings.Config{GroupIDs: []int64{gid}, VerifyMode: settings.ModePoW, DeliveryMode: settings.DeliveryDM})
	v.settings.SetWebCapabilities(settings.WebCapabilities{ConsoleURL: "https://console.example"})
	if got := v.pickMode(gid); got != settings.ModePoW {
		t.Fatalf("available mode=%s", got)
	}
	v.settings.SetWebCapabilities(settings.WebCapabilities{})
	if got := v.pickMode(gid); got != settings.ModeQuiz {
		t.Fatalf("unavailable mode=%s", got)
	}
	group, _ := v.settings.Settings(gid)
	if group.VerifyMode().Value != settings.ModePoW || group.Revision() != 0 {
		t.Fatal("runtime degradation rewrote settings")
	}
}
