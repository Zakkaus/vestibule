package api

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/Zakkaus/vestibule/internal/i18n"
)

// The deployment guide exists so that nobody is asked to paste a credential by
// a screen that has not said where that step sits in the sequence. A page that
// renders the form without the steps is the regression this guards.
func TestSetupGuideShowsFourStepsWithTheCurrentOneMarked(t *testing.T) {
	before := setupPageFor(i18n.LangEN, "", SetupResult{})
	if len(before.Steps) != 4 {
		t.Fatalf("deployment guide rendered %d steps, want the four the design document defines", len(before.Steps))
	}
	wantBefore := []string{"done", "current", "pending", "pending"}
	for index, step := range before.Steps {
		if step.State != wantBefore[index] {
			t.Fatalf("before the claim, step %d is %q, want %q", step.Index, step.State, wantBefore[index])
		}
		if strings.TrimSpace(step.Name) == "" || strings.TrimSpace(step.Note) == "" {
			t.Fatalf("step %d rendered without a name or without the note that says why it happens here", step.Index)
		}
	}

	after := setupPageFor(i18n.LangEN, "", SetupResult{Claimed: true})
	wantAfter := []string{"done", "done", "current", "pending"}
	for index, step := range after.Steps {
		if step.State != wantAfter[index] {
			t.Fatalf("after the claim, step %d is %q, want %q", step.Index, step.State, wantAfter[index])
		}
	}
}

// Telegram refusing a token and Telegram never answering send the reader to two
// different places. Merging them into one sentence sends half of them to
// re-paste a token that was never the problem.
func TestSetupNamesWhichHalfOfAFailedClaimTheReaderCanAct(t *testing.T) {
	setup := i18n.Messages.Bot.Setup
	for name, testCase := range map[string]struct {
		err  error
		want string
	}{
		"token refused":            {err: fmt.Errorf("claim: %w", ErrSetupTokenRejected), want: setup.TokenRejected.For(i18n.LangEN)},
		"telegram unreached":       {err: fmt.Errorf("claim: %w", ErrSetupTelegramUnreachable), want: setup.TelegramUnreachable.For(i18n.LangEN)},
		"instance would-not-start": {err: fmt.Errorf("console link: CONSOLE_URL must be an absolute HTTPS URL or an HTTP URL on a loopback address"), want: setup.InstanceFault.For(i18n.LangEN)},
	} {
		got := claimFailureText(i18n.LangEN, testCase.err)
		if got != testCase.want {
			t.Fatalf("%s rendered %q, want %q", name, got, testCase.want)
		}
	}
	if setup.TokenRejected.For(i18n.LangEN) == setup.TelegramUnreachable.For(i18n.LangEN) ||
		setup.TelegramUnreachable.For(i18n.LangEN) == setup.InstanceFault.For(i18n.LangEN) {
		t.Fatal("two failure causes share one sentence, so the reader cannot tell which one happened")
	}
}

// The claim shows the name Telegram returned so a mispasted token is caught
// here rather than three steps later.
func TestSetupShowsTheBotNameTelegramReturned(t *testing.T) {
	page := setupPageFor(i18n.LangEN, "", SetupResult{Claimed: true, BotUsername: "example_bot"})
	if page.BotName != "@example_bot" {
		t.Fatalf("claimed page showed bot name %q, want @example_bot", page.BotName)
	}
	if setupPageFor(i18n.LangEN, "", SetupResult{Claimed: true, BotUsername: "@example_bot"}).BotName != "@example_bot" {
		t.Fatal("a username that already carries its sigil was rendered with two")
	}
	if setupPageFor(i18n.LangEN, "", SetupResult{Claimed: true}).BotName != "" {
		t.Fatal("a claim that returned no username still rendered a bare sigil")
	}
}

// The stylesheet is checked by the same design gates as the application's, and
// those gates read the rendered markup to tell a used selector from a dead one.
// This test writes that markup, so the fixture cannot drift from the template.
func TestSetupFixtureMatchesTheRenderedPage(t *testing.T) {
	const fixturePath = "setup.css.fixture.html"
	var rendered strings.Builder
	for _, page := range []setupPage{
		setupPageFor(i18n.LangEN, i18n.Messages.Bot.Setup.TokenRejected.For(i18n.LangEN), SetupResult{}),
		setupPageFor(i18n.LangEN, "", SetupResult{Claimed: true, BotUsername: "example_bot", BindingURL: "https://t.me/example_bot?start=owner_x"}),
	} {
		page.Style = ""
		if err := setupPageTemplate.Execute(&rendered, page); err != nil {
			t.Fatalf("rendering the fixture failed: %v", err)
		}
	}
	want := rendered.String()
	got, err := os.ReadFile(fixturePath)
	if err != nil || string(got) != want {
		if os.Getenv("UPDATE_FIXTURES") != "" {
			if writeErr := os.WriteFile(fixturePath, []byte(want), 0o644); writeErr != nil {
				t.Fatalf("writing %s failed: %v", fixturePath, writeErr)
			}
			t.Fatalf("%s was regenerated; commit it", fixturePath)
		}
		t.Fatalf("%s no longer matches the template; regenerate with UPDATE_FIXTURES=1 go test ./internal/console/api/", fixturePath)
	}
}
