package database

import (
	"context"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/Zakkaus/vestibule/internal/verification"
)

func TestAnswerWebRejectsInvalidatedTokens(t *testing.T) {
	for _, invalidation := range []string{"deadline", "rotation", "non-web", "admin settlement"} {
		t.Run(invalidation, func(t *testing.T) {
			f := newWebFixture(t, settings.ModePoW)
			rows, err := f.state.LoadPending("")
			if err != nil {
				t.Fatal(err)
			}
			current := rows[0]
			switch invalidation {
			case "deadline":
				current.Deadline = time.Now().Unix()
				_, err = f.state.UpdatePending("", current.Ref(), current)
			case "rotation":
				_, _, err = f.service.IssueWebToken(context.Background(), verification.ChallengeID(current.Ref()))
			case "non-web":
				current.Mode = settings.ModeQuiz
				_, err = f.state.FallbackWeb(context.Background(), current.Ref(), current, nil)
			case "admin settlement":
				_, err = f.state.TransitionChallenge("", verification.ChallengeTransition{Expected: current.Ref(), Record: current,
					From: verification.ChallengePending, To: verification.ChallengeApproved, SettledAt: time.Now().Unix()})
			}
			if err != nil {
				t.Fatal(err)
			}
			result, err := f.service.AnswerWeb(context.Background(), verification.ChallengeID(current.Ref()), verification.WebProof{Token: f.token, Nonce: solveWebPoW(t, f.proof)})
			if err != nil || result.Outcome != verification.WebSettled {
				t.Fatalf("answer=%#v error=%v", result, err)
			}
			if f.gateway.approvals.Load() != 0 {
				t.Fatal("invalidated bearer approved applicant")
			}
		})
	}
}

func TestWebClaimRechecksDeadlineAndRotation(t *testing.T) {
	for _, mutation := range []string{"deadline", "rotation"} {
		t.Run(mutation, func(t *testing.T) {
			f := newWebFixture(t, settings.ModePoW)
			records, _ := f.state.LoadPending("")
			r := records[0]
			transition := verification.ChallengeTransition{Expected: r.Ref(), Record: r, From: verification.ChallengePending,
				To: verification.ChallengeApproved, SettledAt: time.Now().Unix(), WebClaim: &verification.WebClaim{TokenHash: verification.HashWebToken(f.token), Now: time.Now().Unix()},
				Actions: []verification.ActionIntent{{ID: "web-claim", Kind: "settle_approve", Payload: "{}", NextTryAt: time.Now().Unix()}}}
			if mutation == "deadline" {
				r.Deadline = transition.WebClaim.Now
				_, _ = f.state.UpdatePending("", r.Ref(), r)
			} else {
				if _, _, err := f.service.IssueWebToken(context.Background(), verification.ChallengeID(r.Ref())); err != nil {
					t.Fatal(err)
				}
			}
			changed, err := f.state.TransitionChallenge("", transition)
			if err != nil || changed {
				t.Fatalf("stale proof claimed challenge: %v, %v", changed, err)
			}
			var count int
			if err := f.db.QueryRow(context.Background(), "SELECT COUNT(*) FROM pending_action").Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatal("rejected claim enqueued settlement")
			}
		})
	}
}

func TestWebFallbackKeepsOwnershipAndStartsWindowAfterDelivery(t *testing.T) {
	f := newWebFixture(t, settings.ModeCaptcha)
	records, err := f.state.LoadPending("")
	if err != nil {
		t.Fatal(err)
	}
	old := records[0]
	old.Gate, old.Held, old.HoldUntil, old.Tries = "mute", true, 1999999999, 2
	if _, err := f.state.UpdatePending("", old.Ref(), old); err != nil {
		t.Fatal(err)
	}
	f.service.Shutdown()
	f.restore(t)
	changed, err := f.service.FallbackWeb(context.Background(), verification.ChallengeID(old.Ref()))
	if err != nil || !changed {
		t.Fatalf("fallback: %v, %v", changed, err)
	}
	records, err = f.state.LoadPending("")
	if err != nil {
		t.Fatal(err)
	}
	next := records[0]
	assertWebFallbackState(t, f, old, next)
	f.service.SendDMChallenge(context.Background(), old.UserID, "ja", old.GroupID)
	records, err = f.state.LoadPending("")
	if err != nil {
		t.Fatal(err)
	}
	delivered := records[0]
	if delivered.PrivateMsgID == 0 || delivered.FallbackPending || delivered.Deadline <= next.Deadline {
		t.Fatalf("delivered answer window not started: %#v", delivered)
	}
}

func assertWebFallbackState(t *testing.T, f *webFixture, old, next verification.PendingRecord) {
	t.Helper()
	if next.Mode != "quiz" || next.Nonce != old.Nonce || next.Gate != old.Gate || next.Held != old.Held || next.HoldUntil != old.HoldUntil || next.Tries != old.Tries || next.Lang != old.Lang {
		t.Fatalf("fallback changed ownership: %#v", next)
	}
	if next.ChallengeDelivered || !next.FallbackPending {
		t.Fatal("fallback started answer window before delivery")
	}
	var kind string
	var tokens int
	if err := f.db.QueryRow(context.Background(), "SELECT kind FROM challenge").Scan(&kind); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow(context.Background(), "SELECT COUNT(*) FROM verify_tokens").Scan(&tokens); err != nil {
		t.Fatal(err)
	}
	if kind != "rule" || tokens != 0 {
		t.Fatalf("fallback kind=%s tokens=%d", kind, tokens)
	}
}
