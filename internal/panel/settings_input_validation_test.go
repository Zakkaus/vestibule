package panel

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/mymmrac/telego"
)

func TestPanelDurationParserSupportsEveryUnitAndRejectsInvalidNumbers(t *testing.T) {
	valid := []struct {
		input string
		want  int
	}{
		{input: "1s", want: 30},
		{input: "2m", want: 120},
		{input: "2h", want: 7200},
		{input: "1d", want: 86400},
		{input: "0", want: 0},
	}
	for _, test := range valid {
		if got, ok := parsePanelBanDuration(test.input); !ok || got != test.want {
			t.Errorf("duration %q = %d, %v; want %d, true", test.input, got, ok, test.want)
		}
	}
	for _, input := range []string{"", "not-a-duration", "-1", "2147483649"} {
		if got, ok := parsePanelBanDuration(input); ok {
			t.Errorf("invalid duration %q was accepted as %d", input, got)
		}
	}
}

func TestPanelBoundedNumberParsersEnforceLimitsAndOff(t *testing.T) {
	for _, test := range []struct {
		input        string
		minimum      int
		maximum      int
		want         int
		wantAccepted bool
	}{
		{input: " 1 ", minimum: 1, maximum: 10, want: 1, wantAccepted: true},
		{input: "10", minimum: 1, maximum: 10, want: 10, wantAccepted: true},
		{input: "0", minimum: 1, maximum: 10},
		{input: "11", minimum: 1, maximum: 10, want: 11},
		{input: "invalid", minimum: 1, maximum: 10},
	} {
		got, accepted := parseBoundedPositive(test.input, test.minimum, test.maximum)
		if got != test.want || accepted != test.wantAccepted {
			t.Errorf("bounded number %q = %d, %v; want %d, %v", test.input, got, accepted, test.want, test.wantAccepted)
		}
	}
	for _, input := range []string{"off", " OFF ", "1"} {
		if _, ok := parsePositiveOrOff(input); !ok {
			t.Errorf("positive-or-off input %q was refused", input)
		}
	}
	for _, input := range []string{"0", "invalid"} {
		if got, ok := parsePositiveOrOff(input); ok {
			t.Errorf("invalid positive-or-off input %q was accepted as %d", input, got)
		}
	}
}

func TestPanelTelegramInviteURLsRequireHTTPSHostPathAndNoSuffixes(t *testing.T) {
	for _, input := range []string{"https://t.me/channel", " https://t.me/+privateinvite "} {
		if !validTelegramURL(input) {
			t.Errorf("valid Telegram URL %q was refused", input)
		}
	}
	for _, input := range []string{
		"https://t.me/%zz",
		"http://t.me/channel",
		"https://example.com/channel",
		"https://t.me/",
		"https://t.me/channel?start=1",
		"https://t.me/channel#fragment",
	} {
		if validTelegramURL(input) {
			t.Errorf("unsafe Telegram URL %q was accepted", input)
		}
	}
}

func TestPanelNumericInputsKeepInvalidValuesPendingAndAcceptValidValues(t *testing.T) {
	tests := []struct {
		name, screen, field, invalid, valid string
		wantError                           string
	}{
		{name: "ban duration", screen: "rt", field: "bd", invalid: "bad", valid: "2m", wantError: i18n.Messages.Panel.Settings.Error.InvalidDuration.For(i18n.LangEN)},
		{name: "lookup ttl", screen: "rt", field: "lt", invalid: "0", valid: "7", wantError: i18n.Messages.Panel.Settings.Error.InvalidNumber.For(i18n.LangEN)},
		{name: "timeout", screen: "vp", field: "to", invalid: "29", valid: "300", wantError: i18n.Messages.Panel.Settings.Error.InvalidNumber.For(i18n.LangEN)},
		{name: "mute duration", screen: "md", field: "ms", invalid: "0", valid: "2m", wantError: i18n.Messages.Panel.Settings.Error.InvalidDuration.For(i18n.LangEN)},
		{name: "malformed mute duration", screen: "md", field: "ms", invalid: "bad", valid: "2m", wantError: i18n.Messages.Panel.Settings.Error.InvalidDuration.For(i18n.LangEN)},
		{name: "warning limit", screen: "md", field: "wl", invalid: "0", valid: "5", wantError: i18n.Messages.Panel.Settings.Error.InvalidNumber.For(i18n.LangEN)},
		{name: "maximum failures", screen: "vp", field: "mf", invalid: "0", valid: "5", wantError: i18n.Messages.Panel.Settings.Error.InvalidNumber.For(i18n.LangEN)},
		{name: "retry cooldown", screen: "vp", field: "rc", invalid: "0", valid: "90", wantError: i18n.Messages.Panel.Settings.Error.InvalidNumber.For(i18n.LangEN)},
		{name: "private query rate", screen: "vp", field: "pr", invalid: "0", valid: "9", wantError: i18n.Messages.Panel.Settings.Error.InvalidNumber.For(i18n.LangEN)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertPanelNumericInputRefused(t, test.screen, test.field, test.invalid, test.wantError)
			assertPanelNumericInputAccepted(t, test.screen, test.field, test.valid)
		})
	}
}

func assertPanelNumericInputRefused(t *testing.T, screen, field, input, wantError string) {
	t.Helper()
	panel, store, caller, bot := newSettingsPanelTest(t, "")
	session := addPanelSession(t, panel, store, panelTestGroupA, screen)
	before, _ := store.Settings(panelTestGroupA)
	wantOverrides := before.Overrides()
	wantRevision := before.Revision()
	invokePanelCallback(t, panel, bot, session, panelTestGroupA, field, "_")
	submitPanelText(t, panel, bot, session, input)
	after, _ := store.Settings(panelTestGroupA)
	if after.Revision() != wantRevision || !reflect.DeepEqual(after.Overrides(), wantOverrides) {
		t.Fatalf("invalid %s input changed settings at revision %d: %+v", field, after.Revision(), after.Overrides())
	}
	if session.pending == nil {
		t.Fatalf("invalid %s input discarded the prompt instead of allowing a correction", field)
	}
	if caller.lastSendText != wantError {
		t.Fatalf("invalid %s input notice = %q, want %q", field, caller.lastSendText, wantError)
	}
}

func assertPanelNumericInputAccepted(t *testing.T, screen, field, input string) {
	t.Helper()
	panel, store, caller, bot := newSettingsPanelTest(t, "")
	session := addPanelSession(t, panel, store, panelTestGroupA, screen)
	before, _ := store.Settings(panelTestGroupA)
	invokePanelCallback(t, panel, bot, session, panelTestGroupA, field, "_")
	submitPanelText(t, panel, bot, session, input)
	after, _ := store.Settings(panelTestGroupA)
	if after.Revision() <= before.Revision() {
		t.Fatalf("valid %s input %q did not commit", field, input)
	}
	if session.pending != nil {
		t.Fatalf("valid %s input retained the prompt", field)
	}
	if caller.lastEditText == "" {
		t.Fatalf("valid %s input did not render its parent screen", field)
	}
}

func TestPanelQuizSaveRequiresQuestionOptionsAndSelectedAnswer(t *testing.T) {
	valid := settings.Question{Q: "Question", Options: []string{"A", "B"}, Answer: 0}
	tests := []struct {
		name     string
		question settings.Question
	}{
		{name: "question", question: settings.Question{Q: " ", Options: []string{"A", "B"}, Answer: 0}},
		{name: "two options", question: settings.Question{Q: "Question", Options: []string{"A"}, Answer: 0}},
		{name: "selected answer", question: settings.Question{Q: "Question", Options: []string{"A", "B"}, Answer: -1}},
		{name: "answer in range", question: settings.Question{Q: "Question", Options: []string{"A", "B"}, Answer: 2}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertQuizSaveRefused(t, test.question)
			assertQuizSaveAccepted(t, valid)
		})
	}
}

func assertQuizSaveRefused(t *testing.T, question settings.Question) {
	t.Helper()
	panel, store, caller, bot := newSettingsPanelTest(t, "")
	session := addPanelSession(t, panel, store, panelTestGroupA, "qd")
	session.quiz = &quizDraft{index: -1, question: question, revision: session.revision}
	invokePanelCallback(t, panel, bot, session, panelTestGroupA, "sv", "_")
	group, _ := store.Settings(panelTestGroupA)
	if len(group.Questions().Value) != 0 {
		t.Fatalf("incomplete quiz was saved: %+v", group.Questions().Value)
	}
	want := i18n.Messages.Panel.Settings.Error.QuestionNeedsOptions.For(i18n.LangEN)
	if caller.lastAnswerText != want || session.quiz == nil {
		t.Fatalf("incomplete quiz refusal = %q, draft retained=%v; want %q", caller.lastAnswerText, session.quiz != nil, want)
	}
}

func assertQuizSaveAccepted(t *testing.T, question settings.Question) {
	t.Helper()
	panel, store, _, bot := newSettingsPanelTest(t, "")
	session := addPanelSession(t, panel, store, panelTestGroupA, "qd")
	session.quiz = &quizDraft{index: -1, question: question, revision: session.revision}
	invokePanelCallback(t, panel, bot, session, panelTestGroupA, "sv", "_")
	group, _ := store.Settings(panelTestGroupA)
	if len(group.Questions().Value) != 1 {
		t.Fatalf("complete quiz was not saved: %+v", group.Questions().Value)
	}
}

func TestPanelFallbackSaveRequiresQuestionAndAnswer(t *testing.T) {
	valid := settings.ShortQuestion{Q: "Question", Answers: []string{"Answer"}}
	for _, question := range []settings.ShortQuestion{
		{Q: " ", Answers: []string{"Answer"}},
		{Q: "Question"},
	} {
		panel, store, caller, bot := newSettingsPanelTest(t, "")
		session := addPanelSession(t, panel, store, panelTestGroupA, "fd")
		session.fallback = &fallbackDraft{index: -1, question: question, revision: session.revision}
		invokePanelCallback(t, panel, bot, session, panelTestGroupA, "sv", "_")
		group, _ := store.Settings(panelTestGroupA)
		want := i18n.Messages.Panel.Settings.Error.FallbackNeedsAnswer.For(i18n.LangEN)
		if !group.FallbackBuiltin().Value || caller.lastAnswerText != want || session.fallback == nil {
			t.Fatalf("incomplete fallback changed bank or prompt: builtin=%v notice=%q retained=%v", group.FallbackBuiltin().Value, caller.lastAnswerText, session.fallback != nil)
		}
	}
	panel, store, _, bot := newSettingsPanelTest(t, "")
	session := addPanelSession(t, panel, store, panelTestGroupA, "fd")
	session.fallback = &fallbackDraft{index: -1, question: valid, revision: session.revision}
	invokePanelCallback(t, panel, bot, session, panelTestGroupA, "sv", "_")
	group, _ := store.Settings(panelTestGroupA)
	if group.FallbackBuiltin().Value || len(group.FallbackQuestions().Value) != 1 {
		t.Fatalf("complete fallback was not saved: builtin=%v questions=%+v", group.FallbackBuiltin().Value, group.FallbackQuestions().Value)
	}
}

func TestPanelDraftOptionRemovalPreservesAnswerMeaningAndStoredBanks(t *testing.T) {
	t.Run("quiz", func(t *testing.T) {
		panel, store, _, bot := newSettingsPanelTest(t, "")
		seedPanelBanks(t, store)
		session := addPanelSession(t, panel, store, panelTestGroupA, "qb")
		invokePanelCallback(t, panel, bot, session, panelTestGroupA, "qq", encodeUnsigned(0))
		session.quiz.question.Options[0] = "draft-only"
		group, _ := store.Settings(panelTestGroupA)
		if got := group.Questions().Value[0].Options[0]; got != "A" {
			t.Fatalf("opening a quiz draft aliased stored options: %q", got)
		}
		session.quiz.question.Options = []string{"first", "correct", "last"}
		session.quiz.question.Answer = 1
		invokePanelCallback(t, panel, bot, session, panelTestGroupA, "dl", encodeUnsigned(0))
		if got := session.quiz.question; got.Answer != 0 || !reflect.DeepEqual(got.Options, []string{"correct", "last"}) {
			t.Fatalf("deleting an earlier option changed answer meaning: %+v", got)
		}
		invokePanelCallback(t, panel, bot, session, panelTestGroupA, "dl", encodeUnsigned(0))
		if session.quiz.question.Answer != -1 {
			t.Fatalf("deleting the selected option left answer index %d", session.quiz.question.Answer)
		}
		group, _ = store.Settings(panelTestGroupA)
		if got := group.Questions().Value[0].Options; !reflect.DeepEqual(got, []string{"A", "B"}) {
			t.Fatalf("editing a quiz draft mutated stored options: %v", got)
		}
	})
	t.Run("fallback", func(t *testing.T) {
		panel, store, _, bot := newSettingsPanelTest(t, "")
		seedPanelBanks(t, store)
		session := addPanelSession(t, panel, store, panelTestGroupA, "fb")
		invokePanelCallback(t, panel, bot, session, panelTestGroupA, "fq", encodeUnsigned(0))
		session.fallback.question.Answers[0] = "draft-only"
		group, _ := store.Settings(panelTestGroupA)
		if got := group.FallbackQuestions().Value[0].Answers[0]; got != "Answer" {
			t.Fatalf("opening a fallback draft aliased stored answers: %q", got)
		}
		session.fallback.question.Answers = []string{"first", "second"}
		invokePanelCallback(t, panel, bot, session, panelTestGroupA, "dl", encodeUnsigned(0))
		if !reflect.DeepEqual(session.fallback.question.Answers, []string{"second"}) {
			t.Fatalf("fallback answer deletion = %v", session.fallback.question.Answers)
		}
		group, _ = store.Settings(panelTestGroupA)
		if got := group.FallbackQuestions().Value[0].Answers; !reflect.DeepEqual(got, []string{"Answer"}) {
			t.Fatalf("editing a fallback draft mutated stored answers: %v", got)
		}
	})
}

func TestPanelContentBanksNavigateAndPageThroughEveryQuestion(t *testing.T) {
	panel, store, caller, bot := newSettingsPanelTest(t, "")
	group, _ := store.Settings(panelTestGroupA)
	questions := make([]settings.Question, panelPageSize+1)
	fallback := make([]settings.ShortQuestion, panelPageSize+1)
	for index := range questions {
		questions[index] = settings.Question{Q: "Quiz " + encodeUnsigned(uint64(index+1)), Options: []string{"A", "B"}, Answer: 0}
		fallback[index] = settings.ShortQuestion{Q: "Fallback " + encodeUnsigned(uint64(index+1)), Answers: []string{"A"}}
	}
	custom := false
	next := group.Overrides()
	next.Questions, next.FallbackQuestions, next.FallbackBuiltin = &questions, &fallback, &custom
	if _, err := store.Update(panelTestGroupA, group.Revision(), next); err != nil {
		t.Fatal(err)
	}
	for _, screen := range []string{"qb", "fb"} {
		session := addPanelSession(t, panel, store, panelTestGroupA, screen)
		invokePanelCallback(t, panel, bot, session, panelTestGroupA, "pg", encodeUnsigned(1))
		if session.page != 1 || !strings.Contains(caller.lastEditText, encodeUnsigned(panelPageSize+1)) {
			t.Fatalf("%s second page = page %d text %q", screen, session.page, caller.lastEditText)
		}
		invokePanelCallback(t, panel, bot, session, panelTestGroupA, "go", "ct")
		if session.screen != "ct" {
			t.Fatalf("%s back action left screen %q", screen, session.screen)
		}
	}
	session := addPanelSession(t, panel, store, panelTestGroupA, "ch")
	invokePanelCallback(t, panel, bot, session, panelTestGroupA, "go", "ct")
	if session.screen != "ct" {
		t.Fatalf("channel back action left screen %q", session.screen)
	}
}

func TestPanelBankSelectionsRejectOutOfRangeIndexesAndAcceptValidOnes(t *testing.T) {
	t.Run("quiz", func(t *testing.T) {
		panel, store, caller, bot := newSettingsPanelTest(t, "")
		seedPanelBanks(t, store)
		session := addPanelSession(t, panel, store, panelTestGroupA, "qb")
		invokePanelCallback(t, panel, bot, session, panelTestGroupA, "qq", encodeUnsigned(1))
		assertPanelConflictNotice(t, caller)
		if session.quiz != nil {
			t.Fatalf("out-of-range quiz index opened draft %+v", session.quiz)
		}

		panel, store, _, bot = newSettingsPanelTest(t, "")
		seedPanelBanks(t, store)
		session = addPanelSession(t, panel, store, panelTestGroupA, "qb")
		invokePanelCallback(t, panel, bot, session, panelTestGroupA, "qq", encodeUnsigned(0))
		if session.quiz == nil || session.quiz.index != 0 {
			t.Fatalf("valid quiz index did not open question zero: %+v", session.quiz)
		}
	})
	t.Run("fallback", func(t *testing.T) {
		panel, store, caller, bot := newSettingsPanelTest(t, "")
		seedPanelBanks(t, store)
		session := addPanelSession(t, panel, store, panelTestGroupA, "fb")
		invokePanelCallback(t, panel, bot, session, panelTestGroupA, "fq", encodeUnsigned(1))
		assertPanelConflictNotice(t, caller)
		if session.fallback != nil {
			t.Fatalf("out-of-range fallback index opened draft %+v", session.fallback)
		}

		panel, store, _, bot = newSettingsPanelTest(t, "")
		seedPanelBanks(t, store)
		session = addPanelSession(t, panel, store, panelTestGroupA, "fb")
		invokePanelCallback(t, panel, bot, session, panelTestGroupA, "fq", encodeUnsigned(0))
		if session.fallback == nil || session.fallback.index != 0 {
			t.Fatalf("valid fallback index did not open question zero: %+v", session.fallback)
		}
	})
}

func TestPanelDraftItemActionsRejectOutOfRangeIndexesAndAcceptValidOnes(t *testing.T) {
	t.Run("select quiz answer", func(t *testing.T) {
		panel, store, caller, bot := newSettingsPanelTest(t, "")
		session := addPanelSession(t, panel, store, panelTestGroupA, "qd")
		session.quiz = panelQuizDraft(session, 0)
		invokePanelCallback(t, panel, bot, session, panelTestGroupA, "ok", encodeUnsigned(2))
		assertPanelConflictNotice(t, caller)
		if session.quiz.question.Answer != 0 {
			t.Fatalf("out-of-range option selected answer %d", session.quiz.question.Answer)
		}

		panel, store, _, bot = newSettingsPanelTest(t, "")
		session = addPanelSession(t, panel, store, panelTestGroupA, "qd")
		session.quiz = panelQuizDraft(session, 0)
		invokePanelCallback(t, panel, bot, session, panelTestGroupA, "ok", encodeUnsigned(1))
		if session.quiz.question.Answer != 1 {
			t.Fatalf("valid option left answer %d", session.quiz.question.Answer)
		}
	})
	t.Run("delete quiz option", func(t *testing.T) {
		panel, store, caller, bot := newSettingsPanelTest(t, "")
		session := addPanelSession(t, panel, store, panelTestGroupA, "qd")
		session.quiz = panelQuizDraft(session, 0)
		invokePanelCallback(t, panel, bot, session, panelTestGroupA, "dl", encodeUnsigned(2))
		assertPanelConflictNotice(t, caller)
		if len(session.quiz.question.Options) != 2 {
			t.Fatalf("out-of-range deletion left %d quiz options", len(session.quiz.question.Options))
		}

		panel, store, _, bot = newSettingsPanelTest(t, "")
		session = addPanelSession(t, panel, store, panelTestGroupA, "qd")
		session.quiz = panelQuizDraft(session, 0)
		invokePanelCallback(t, panel, bot, session, panelTestGroupA, "dl", encodeUnsigned(1))
		if !reflect.DeepEqual(session.quiz.question.Options, []string{"A"}) {
			t.Fatalf("valid quiz deletion left options %v", session.quiz.question.Options)
		}
	})
}

func TestPanelFallbackAnswerDeletionRejectsOutOfRangeAndAcceptsValidIndex(t *testing.T) {
	panel, store, caller, bot := newSettingsPanelTest(t, "")
	session := addPanelSession(t, panel, store, panelTestGroupA, "fd")
	session.fallback = panelFallbackDraft(session)
	invokePanelCallback(t, panel, bot, session, panelTestGroupA, "dl", encodeUnsigned(2))
	assertPanelConflictNotice(t, caller)
	if len(session.fallback.question.Answers) != 2 {
		t.Fatalf("out-of-range deletion left %d fallback answers", len(session.fallback.question.Answers))
	}

	panel, store, _, bot = newSettingsPanelTest(t, "")
	session = addPanelSession(t, panel, store, panelTestGroupA, "fd")
	session.fallback = panelFallbackDraft(session)
	invokePanelCallback(t, panel, bot, session, panelTestGroupA, "dl", encodeUnsigned(1))
	if !reflect.DeepEqual(session.fallback.question.Answers, []string{"A"}) {
		t.Fatalf("valid fallback deletion left answers %v", session.fallback.question.Answers)
	}
}

func panelQuizDraft(session *panelSession, answer int) *quizDraft {
	return &quizDraft{
		index: -1, revision: session.revision,
		question: settings.Question{Q: "Question", Options: []string{"A", "B"}, Answer: answer},
	}
}

func panelFallbackDraft(session *panelSession) *fallbackDraft {
	return &fallbackDraft{
		index: -1, revision: session.revision,
		question: settings.ShortQuestion{Q: "Question", Answers: []string{"A", "B"}},
	}
}

func assertPanelConflictNotice(t *testing.T, caller *panelAPICaller) {
	t.Helper()
	want := i18n.Messages.Panel.Settings.Error.ConcurrentChange.For(i18n.LangEN)
	if caller.lastAnswerText != want {
		t.Fatalf("unsafe indexed action notice = %q, want conflict %q", caller.lastAnswerText, want)
	}
}

func TestPanelNewDraftsCannotBeDeletedButExistingDraftsCan(t *testing.T) {
	for _, kind := range []string{"quiz", "fallback"} {
		t.Run(kind, func(t *testing.T) {
			panel, store, caller, bot := newSettingsPanelTest(t, "")
			screen := "qd"
			session := addPanelSession(t, panel, store, panelTestGroupA, screen)
			session.quiz = panelQuizDraft(session, 0)
			if kind == "fallback" {
				session.screen = "fd"
				session.quiz = nil
				session.fallback = panelFallbackDraft(session)
			}
			invokePanelCallback(t, panel, bot, session, panelTestGroupA, "rm", "_")
			if session.confirm != nil {
				t.Fatalf("new %s draft opened destructive confirmation %+v", kind, session.confirm)
			}
			want := i18n.Messages.Panel.Settings.Error.SaveFailed.For(i18n.LangEN)
			if caller.lastAnswerText != want {
				t.Fatalf("new %s deletion notice = %q, want refusal %q", kind, caller.lastAnswerText, want)
			}

			panel, store, _, bot = newSettingsPanelTest(t, "")
			seedPanelBanks(t, store)
			bankScreen, openField := "qb", "qq"
			if kind == "fallback" {
				bankScreen, openField = "fb", "fq"
			}
			session = addPanelSession(t, panel, store, panelTestGroupA, bankScreen)
			invokePanelCallback(t, panel, bot, session, panelTestGroupA, openField, encodeUnsigned(0))
			invokePanelCallback(t, panel, bot, session, panelTestGroupA, "rm", "_")
			if session.confirm == nil || session.screen != "cf" {
				t.Fatalf("existing %s draft did not open deletion confirmation", kind)
			}
		})
	}
}

func TestPanelInviteEditingRequiresAConfiguredChannelAndValidTelegramURL(t *testing.T) {
	panel, store, caller, bot := newSettingsPanelTest(t, "")
	session := addPanelSession(t, panel, store, panelTestGroupA, "ch")
	invokePanelCallback(t, panel, bot, session, panelTestGroupA, "iu", "_")
	if session.pending != nil {
		t.Fatal("invite editing without a required channel armed an input prompt")
	}
	wantChat := i18n.Messages.Panel.Settings.Error.InvalidChat.For(i18n.LangEN)
	if caller.lastAnswerText != wantChat {
		t.Fatalf("missing-channel invite notice = %q, want %q", caller.lastAnswerText, wantChat)
	}

	const channelID int64 = -1009000000911
	panel, store, caller, bot = newSettingsPanelTest(t, "")
	seedPanelChannel(t, store, channelID, "@channel", "")
	session = addPanelSession(t, panel, store, panelTestGroupA, "ch")
	invokePanelCallback(t, panel, bot, session, panelTestGroupA, "iu", "_")
	before, _ := store.Settings(panelTestGroupA)
	submitPanelText(t, panel, bot, session, "https://example.com/invite")
	after, _ := store.Settings(panelTestGroupA)
	if !reflect.DeepEqual(after.Overrides(), before.Overrides()) || session.pending == nil {
		t.Fatalf("invalid invite changed channel or discarded prompt: %+v", after.Overrides())
	}
	wantURL := i18n.Messages.Panel.Settings.Error.InvalidURL.For(i18n.LangEN)
	if caller.lastSendText != wantURL {
		t.Fatalf("invalid invite notice = %q, want %q", caller.lastSendText, wantURL)
	}
	submitPanelText(t, panel, bot, session, "https://t.me/channel")
	after, _ = store.Settings(panelTestGroupA)
	if after.ChannelInviteURL().Value != "https://t.me/channel" {
		t.Fatalf("valid Telegram invite was not stored: %q", after.ChannelInviteURL().Value)
	}
}

func TestPanelOnlyPublicChannelInvitesCanBeCleared(t *testing.T) {
	const channelID int64 = -1009000000912
	panel, store, caller, bot := newSettingsPanelTest(t, "")
	privateWant := seedPanelChannel(t, store, channelID, "Private channel", "https://t.me/+private")
	session := addPanelSession(t, panel, store, panelTestGroupA, "ch")
	invokePanelCallback(t, panel, bot, session, panelTestGroupA, "dl", "_")
	assertGroupOverrides(t, store, privateWant)
	want := i18n.Messages.Panel.Settings.Error.InvalidURL.For(i18n.LangEN)
	if caller.lastAnswerText != want {
		t.Fatalf("private invite clear notice = %q, want %q", caller.lastAnswerText, want)
	}

	panel, store, _, bot = newSettingsPanelTest(t, "")
	seedPanelChannel(t, store, channelID, "@public", "https://t.me/public")
	session = addPanelSession(t, panel, store, panelTestGroupA, "ch")
	invokePanelCallback(t, panel, bot, session, panelTestGroupA, "dl", "_")
	group, _ := store.Settings(panelTestGroupA)
	if group.ChannelInviteURL().Value != "" {
		t.Fatalf("public invite clear left %q", group.ChannelInviteURL().Value)
	}
}

func TestPanelStaleDraftAndConfirmationCancellationRefusesConcurrentChanges(t *testing.T) {
	for _, kind := range []string{"quiz", "fallback", "confirmation"} {
		t.Run(kind, func(t *testing.T) {
			panel, store, caller, bot := newSettingsPanelTest(t, "")
			seedPanelBanks(t, store)
			session := preparePanelCancelState(t, panel, store, bot, kind)
			bumpPanelGroupRevision(t, store)
			invokePanelCallback(t, panel, bot, session, panelTestGroupA, "cn", "_")
			assertPanelConflictNotice(t, caller)
			if panel.sessionByUser(panelTestUser) != nil {
				t.Fatalf("stale %s cancellation retained its session", kind)
			}

			panel, store, _, bot = newSettingsPanelTest(t, "")
			seedPanelBanks(t, store)
			session = preparePanelCancelState(t, panel, store, bot, kind)
			invokePanelCallback(t, panel, bot, session, panelTestGroupA, "cn", "_")
			if panel.sessionByUser(panelTestUser) == nil {
				t.Fatalf("current %s cancellation was refused", kind)
			}
		})
	}
}

func preparePanelCancelState(t *testing.T, panel *Panel, store *settings.Store, bot *telego.Bot, kind string) *panelSession {
	t.Helper()
	screen, field := "qb", "qq"
	if kind == "fallback" {
		screen, field = "fb", "fq"
	}
	session := addPanelSession(t, panel, store, panelTestGroupA, screen)
	invokePanelCallback(t, panel, bot, session, panelTestGroupA, field, encodeUnsigned(0))
	if kind == "confirmation" {
		invokePanelCallback(t, panel, bot, session, panelTestGroupA, "rm", "_")
	}
	return session
}

func TestPanelBuiltinFallbackBankCannotOpenAQuestionButCustomBankCan(t *testing.T) {
	panel, store, caller, bot := newSettingsPanelTest(t, "")
	session := addPanelSession(t, panel, store, panelTestGroupA, "fb")
	invokePanelCallback(t, panel, bot, session, panelTestGroupA, "fq", encodeUnsigned(0))
	assertPanelConflictNotice(t, caller)
	if session.fallback != nil {
		t.Fatalf("built-in fallback bank opened editable draft %+v", session.fallback)
	}

	panel, store, _, bot = newSettingsPanelTest(t, "")
	seedPanelBanks(t, store)
	session = addPanelSession(t, panel, store, panelTestGroupA, "fb")
	invokePanelCallback(t, panel, bot, session, panelTestGroupA, "fq", encodeUnsigned(0))
	if session.fallback == nil {
		t.Fatal("custom fallback bank did not open its first question")
	}
}
