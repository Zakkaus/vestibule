package database

import (
	"context"
	"testing"

	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/Zakkaus/vestibule/internal/verification"
)

func TestRestoredWebCapabilityLossTransitionsWithoutRewritingSettings(t *testing.T) {
	f := newWebFixture(t, settings.ModeCaptcha)
	f.service.Shutdown()
	f.restoreWithCapabilities(t, settings.WebCapabilities{ConsoleURL: "https://console.example"})
	view, _ := f.settings.Settings(f.record.GroupID)
	if view.VerifyMode().Value != settings.ModeCaptcha || view.Revision() != 0 {
		t.Fatal("capability loss rewrote settings")
	}
	records, err := f.state.LoadPending("")
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].Mode != settings.ModeQuiz || records[0].Nonce != f.record.Nonce {
		t.Fatalf("restored fallback=%#v", records)
	}
	_, _, valid, err := f.service.ResolveWebToken(context.Background(), f.token)
	if err != nil || valid {
		t.Fatalf("fallback token accepted: %v, %v", valid, err)
	}
}

func TestWebFallbackEmptyQuizBankUsesKernelNotFallbackBank(t *testing.T) {
	f := newWebFixture(t, settings.ModeCaptcha)
	view, _ := f.settings.Settings(f.record.GroupID)
	next := view.Overrides()
	empty := []settings.Question{}
	alternative := []settings.ShortQuestion{{Q: "Do not select this bank", Answers: []string{"answer"}}}
	builtin := false
	next.Questions, next.FallbackQuestions, next.FallbackBuiltin = &empty, &alternative, &builtin
	if _, err := f.settings.Update(f.record.GroupID, view.Revision(), next); err != nil {
		t.Fatal(err)
	}
	changed, err := f.service.FallbackWeb(context.Background(), verification.ChallengeID(f.record.Ref()))
	if err != nil || !changed {
		t.Fatalf("empty-bank fallback=%v, %v", changed, err)
	}
	records, err := f.state.LoadPending("")
	if err != nil {
		t.Fatal(err)
	}
	if records[0].Mode != settings.ModeKernel || len(records[0].FbAnswers) != 0 || records[0].QText == alternative[0].Q {
		t.Fatalf("empty bank selected non-kernel fallback: %#v", records[0])
	}
}
