package lookup_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/i18n"
	"github.com/Zakkaus/vestibule/internal/lookup"
	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/Zakkaus/vestibule/internal/telegram"
	"github.com/Zakkaus/vestibule/internal/telegram/tgfmt"
	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
)

func TestAllLookupCommandHandlersSendCatalogueAnswers(t *testing.T) {
	newsItems := []lookup.NewsItem{{Date: "2026-08-24", Title: "Kernel update", URL: "https://example.test/kernel"}}
	lookup.TestWithFreshNews(t, newsItems)

	tests := []struct {
		Name      string
		Text      string
		Handler   func(*telegram.LookupHandlers) th.Handler
		Want      string
		ParseMode string
	}{
		{Name: "pkg", Text: "/gpkg", Handler: func(service *telegram.LookupHandlers) th.Handler { return service.OnPkg }, Want: i18n.Messages.LookupPackages.Pkg.Usage.For(i18n.LangEN)},
		{Name: "use", Text: "/guse", Handler: func(service *telegram.LookupHandlers) th.Handler { return service.OnUse }, Want: i18n.Messages.LookupPackages.Use.Usage.For(i18n.LangEN)},
		{Name: "bug", Text: "/gbug", Handler: func(service *telegram.LookupHandlers) th.Handler { return service.OnBug }, Want: i18n.Messages.LookupContent.Bug.Usage.For(i18n.LangEN)},
		{Name: "news", Text: "/gnews", Handler: func(service *telegram.LookupHandlers) th.Handler { return service.OnNews }, Want: tgfmt.RenderNews(i18n.LangEN, "", newsItems, true), ParseMode: telego.ModeHTML},
		{Name: "wiki", Text: "/wiki", Handler: func(service *telegram.LookupHandlers) th.Handler { return service.OnWiki }, Want: i18n.Messages.LookupContent.Wiki.Usage.For(i18n.LangEN)},
		{Name: "bbs", Text: "/gbbs", Handler: func(service *telegram.LookupHandlers) th.Handler { return service.OnBbs }, Want: i18n.Messages.LookupContent.BBS.Usage.For(i18n.LangEN)},
		{Name: "pkgs", Text: "/pkgs", Handler: func(service *telegram.LookupHandlers) th.Handler { return service.OnPkgs }, Want: i18n.Messages.LookupDistros.Pkgs.Usage.For(i18n.LangEN)},
		{Name: "distro alias", Text: "/distro", Handler: func(service *telegram.LookupHandlers) th.Handler { return service.OnPkgs }, Want: i18n.Messages.LookupDistros.Pkgs.Usage.For(i18n.LangEN)},
		{Name: "arm", Text: "/garm", Handler: func(service *telegram.LookupHandlers) th.Handler { return service.OnArm }, Want: i18n.Messages.LookupPackages.Arm.Usage.For(i18n.LangEN)},
		{Name: "armpkgs", Text: "/armpkgs", Handler: func(service *telegram.LookupHandlers) th.Handler { return service.OnArmpkgs }, Want: i18n.Messages.LookupDistros.Armpkgs.Usage.For(i18n.LangEN)},
	}
	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			caller := &lookup.TestLookupTelegramCaller{}
			bot := lookup.TestNewLookupTestBot(t, caller)
			service := newLookupTestService(nil, telegram.NewConnector(bot), &settings.Config{}, "")
			lookup.TestRunLookupHandler(t, bot, tt.Handler(service), lookup.TestLookupMessage(tt.Text, 77, 77, telego.ChatTypePrivate))
			if got := len(caller.MethodCalls("sendMessage")); got != 1 {
				t.Fatalf("sendMessage calls = %d, want 1", got)
			}
			params := lookup.TestSentLookupMessage(t, caller, 0)
			if params.Text != tt.Want || params.ParseMode != tt.ParseMode || params.ChatID.ID != 77 ||
				params.ReplyParameters == nil || params.ReplyParameters.MessageID != 41 {
				t.Fatalf("outbound answer = text %q parse_mode %q chat %d reply %+v; want catalogue text %q parse_mode %q chat 77 reply 41",
					params.Text, params.ParseMode, params.ChatID.ID, params.ReplyParameters, tt.Want, tt.ParseMode)
			}
		})
	}
}

func TestPkgsExcludesDebianTestingFromTheStableLine(t *testing.T) {
	(*lookup.TestRelInfo).Mu.Lock()
	oldDebian, oldFetched, oldRefreshing := (*lookup.TestRelInfo).Debian, (*lookup.TestRelInfo).Fetched, (*lookup.TestRelInfo).Refreshing
	(*lookup.TestRelInfo).Debian = map[string]string{"13": "stable", "14": "testing"}
	(*lookup.TestRelInfo).Fetched, (*lookup.TestRelInfo).Refreshing = time.Now(), false
	(*lookup.TestRelInfo).Mu.Unlock()
	t.Cleanup(func() {
		(*lookup.TestRelInfo).Mu.Lock()
		(*lookup.TestRelInfo).Debian, (*lookup.TestRelInfo).Fetched, (*lookup.TestRelInfo).Refreshing = oldDebian, oldFetched, oldRefreshing
		(*lookup.TestRelInfo).Mu.Unlock()
	})

	lookup.TestWithLookupHTTP(t, func(request *http.Request) (int, string) {
		switch request.URL.Host {
		case "repology.org":
			return http.StatusOK, `[{"repo":"debian_13","version":"1.0"},{"repo":"debian_14","version":"2.0"}]`
		case "packages.gentoo.org":
			return http.StatusOK, ""
		default:
			return http.StatusNotFound, ""
		}
	})
	const chatID = int64(-1009000000017)
	caller := &lookup.TestLookupTelegramCaller{}
	bot := lookup.TestNewLookupTestBot(t, caller)
	service := newLookupTestService(nil, telegram.NewConnector(bot), &settings.Config{
		Groups:   []settings.GroupConfig{{ID: chatID, Lang: "en"}},
		GroupIDs: []int64{chatID},
	}, "")

	lookup.TestRunLookupHandler(t, bot, service.OnPkgs, lookup.TestLookupMessage("/pkgs demo", chatID, 88, telego.ChatTypeSupergroup))
	if got := len(caller.MethodCalls("sendMessage")); got != 1 {
		t.Fatalf("Debian package lookup sent %d replies, want 1", got)
	}
	text := lookup.TestSentLookupMessage(t, caller, 0).Text
	if !strings.Contains(text, "1.0") || !strings.Contains(text, "13 stable") {
		t.Fatalf("Debian testing replaced the stable line: %q; people cannot install testing versions on stable Debian", text)
	}
	if strings.Contains(text, "2.0") || strings.Contains(text, "14 testing") {
		t.Fatalf("Debian testing leaked into the stable line: %q; people cannot install testing versions on stable Debian", text)
	}
}

func TestLookupHandlerArguments(t *testing.T) {
	t.Run("missing update data is ignored", func(t *testing.T) {
		caller := &lookup.TestLookupTelegramCaller{}
		bot := lookup.TestNewLookupTestBot(t, caller)
		service := newLookupTestService(nil, telegram.NewConnector(bot), &settings.Config{}, "")
		lookup.TestRunLookupHandler(t, bot, service.OnPkg, telego.Update{})
		withoutSender := lookup.TestLookupMessage("/gpkg vim", 77, 77, telego.ChatTypePrivate)
		withoutSender.Message.From = nil
		lookup.TestRunLookupHandler(t, bot, service.OnPkg, withoutSender)
		if got := len(caller.MethodCalls("sendMessage")); got != 0 {
			t.Fatalf("missing message or sender produced %d replies", got)
		}
	})

	t.Run("extra bug argument is rejected from the catalogue", func(t *testing.T) {
		caller := &lookup.TestLookupTelegramCaller{}
		bot := lookup.TestNewLookupTestBot(t, caller)
		service := newLookupTestService(nil, telegram.NewConnector(bot), &settings.Config{}, "")
		lookup.TestRunLookupHandler(t, bot, service.OnBug, lookup.TestLookupMessage("/gbug 123 extra", 77, 77, telego.ChatTypePrivate))
		want := i18n.Messages.LookupContent.Bug.Usage.For(i18n.LangEN)
		if got := lookup.TestSentLookupMessage(t, caller, 0).Text; got != want {
			t.Fatalf("extra-argument answer = %q, want catalogue usage %q", got, want)
		}
	})

	t.Run("quoted news query reaches the renderer intact", func(t *testing.T) {
		items := []lookup.NewsItem{{Date: "2026-08-24", Title: "Kernel update", URL: "https://example.test/kernel"}}
		lookup.TestWithFreshNews(t, items)
		caller := &lookup.TestLookupTelegramCaller{}
		bot := lookup.TestNewLookupTestBot(t, caller)
		service := newLookupTestService(nil, telegram.NewConnector(bot), &settings.Config{}, "")
		const arg = `"kernel update"`
		lookup.TestRunLookupHandler(t, bot, service.OnNews, lookup.TestLookupMessage("/gnews "+arg, 77, 77, telego.ChatTypePrivate))
		want := tgfmt.RenderNews(i18n.LangEN, arg, items, true)
		if got := lookup.TestSentLookupMessage(t, caller, 0); got.Text != want || got.ParseMode != telego.ModeHTML {
			t.Fatalf("quoted-query answer = text %q parse_mode %q, want catalogue rendering %q in HTML", got.Text, got.ParseMode, want)
		}
	})

}

func TestLookupRichMessageFallbacks(t *testing.T) {
	for _, tt := range []struct {
		Name  string
		Query string
		Err   error
	}{
		{Name: "category atom and oversized rich message", Query: "app-editors/vim", Err: errors.New("Bad Request: message is too long")},
		{Name: "bare atom and rejected rich HTML", Query: "vim", Err: errors.New("Bad Request: can't parse entities")},
	} {
		t.Run(tt.Name, func(t *testing.T) {
			lookup.TestResetLookupPackageCaches(t)
			lookup.TestWithLookupHTTP(t, func(request *http.Request) (int, string) {
				if request.URL.Host == "api.github.com" {
					return http.StatusOK, `{"tree":[]}`
				}
				if strings.HasSuffix(request.URL.Path, ".json") {
					return http.StatusOK, `{"versions":[]}`
				}
				return http.StatusOK, "<html></html>"
			})
			caller := &lookup.TestLookupTelegramCaller{Responses: map[string][]lookup.TestLookupTelegramResult{
				"sendRichMessage": {{Err: tt.Err}},
			}}
			bot := lookup.TestNewLookupTestBot(t, caller)
			service := newLookupTestService(nil, telegram.NewConnector(bot), &settings.Config{
				RichMessages: true,
				Overlays:     []settings.OverlayCfg{{Name: "test", Repo: "test/repo"}},
			}, "")
			lookup.TestRunLookupHandler(t, bot, service.OnPkg, lookup.TestLookupMessage("/gpkg "+tt.Query, 77, 77, telego.ChatTypePrivate))
			if got := len(caller.MethodCalls("sendRichMessage")); got != 1 {
				t.Fatalf("sendRichMessage calls = %d, want 1", got)
			}
			if got := len(caller.MethodCalls("sendMessage")); got != 1 {
				t.Fatalf("HTML fallback calls = %d, want 1", got)
			}
			availability := lookup.TestPkgLookupAvailability{Official: true, Overlays: map[string]bool{"test": true}}
			want := tgfmt.RenderPkg(i18n.LangEN, tt.Query, lookup.PackageResult{Official: nil, Versions: map[string][2]string{}, Availability: availability})
			params := lookup.TestSentLookupMessage(t, caller, 0)
			if params.Text != want || params.ParseMode != telego.ModeHTML {
				t.Fatalf("HTML fallback = text %q parse_mode %q, want catalogue rendering %q in HTML", params.Text, params.ParseMode, want)
			}
		})
	}

}

func TestLookupBBSButtonFallbacks(t *testing.T) {
	for _, sendErr := range []error{
		errors.New("Bad Request: message is too long"),
		errors.New("Bad Request: can't parse entities"),
	} {
		t.Run("bbs removes buttons after "+sendErr.Error(), func(t *testing.T) {
			lookup.TestWithLookupHTTP(t, func(*http.Request) (int, string) { return http.StatusOK, `{}` })
			caller := &lookup.TestLookupTelegramCaller{Responses: map[string][]lookup.TestLookupTelegramResult{
				"sendMessage": {{Err: sendErr}, {}},
			}}
			bot := lookup.TestNewLookupTestBot(t, caller)
			service := newLookupTestService(nil, telegram.NewConnector(bot), &settings.Config{}, "")
			const query = "kernel modules"
			lookup.TestRunLookupHandler(t, bot, service.OnBbs, lookup.TestLookupMessage("/gbbs "+query, 77, 77, telego.ChatTypePrivate))
			calls := caller.MethodCalls("sendMessage")
			if len(calls) != 2 {
				t.Fatalf("sendMessage calls = %d, want button send and text-only fallback", len(calls))
			}
			want := i18n.Messages.LookupContent.BBS.Heading.Render(i18n.LangEN, query) +
				i18n.Messages.LookupContent.BBS.ArchCNNoMatches.For(i18n.LangEN) +
				i18n.Messages.LookupContent.BBS.OtherForums.For(i18n.LangEN)
			var first, fallback struct {
				Text        string          `json:"text"`
				ParseMode   string          `json:"parse_mode"`
				ReplyMarkup json.RawMessage `json:"reply_markup"`
			}
			if err := json.Unmarshal(calls[0].Body, &first); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(calls[1].Body, &fallback); err != nil {
				t.Fatal(err)
			}
			if first.Text != want || len(first.ReplyMarkup) == 0 || string(first.ReplyMarkup) == "null" ||
				fallback.Text != want || len(fallback.ReplyMarkup) != 0 || fallback.ParseMode != telego.ModeHTML {
				t.Fatalf("BBS send/fallback = first %+v fallback %+v, want catalogue HTML %q with buttons then without", first, fallback, want)
			}
		})
	}
}

func TestLookupHandlerAuthorizationAndRate(t *testing.T) {
	const (
		groupID = int64(-1009999900013)
		userID  = int64(88)
	)
	limit := 1
	disabledTTL := 0
	cfg := &settings.Config{
		Groups:             []settings.GroupConfig{{ID: groupID, Lang: "en"}},
		GroupIDs:           []int64{groupID},
		PrivateQueryPerMin: limit,
		LookupTTLSeconds:   &disabledTTL,
	}
	caller := &lookup.TestLookupTelegramCaller{}
	bot := lookup.TestNewLookupTestBot(t, caller)
	service := newLookupTestService(nil, telegram.NewConnector(bot), cfg, "")

	lookup.TestRunLookupHandler(t, bot, service.OnWiki, lookup.TestLookupMessage("/wiki", 900, userID, telego.ChatTypeSupergroup))
	if got := len(caller.MethodCalls("sendMessage")); got != 0 {
		t.Fatalf("unguarded group produced %d replies", got)
	}

	lookup.TestRunLookupHandler(t, bot, service.OnWiki, lookup.TestLookupMessage("/wiki", userID, userID, telego.ChatTypePrivate))
	lookup.TestRunLookupHandler(t, bot, service.OnWiki, lookup.TestLookupMessage("/wiki", userID, userID, telego.ChatTypePrivate))
	if got := len(caller.MethodCalls("sendMessage")); got != 2 {
		t.Fatalf("private lookup sends = %d, want usage then rate-limit notice", got)
	}
	if got, want := lookup.TestSentLookupMessage(t, caller, 0).Text, i18n.Messages.LookupContent.Wiki.Usage.For(i18n.LangEN); got != want {
		t.Fatalf("first private lookup = %q, want catalogue usage %q", got, want)
	}
	if got, want := lookup.TestSentLookupMessage(t, caller, 1).Text,
		i18n.Messages.LookupContent.Transport.PrivateRateLimited.Render(i18n.LangEN, limit); got != want {
		t.Fatalf("limited private lookup = %q, want catalogue notice %q", got, want)
	}

	beforeGroup := len(caller.MethodCalls("sendMessage"))
	lookup.TestRunLookupHandler(t, bot, service.OnWiki, lookup.TestLookupMessage("/wiki", groupID, userID, telego.ChatTypeSupergroup))
	lookup.TestRunLookupHandler(t, bot, service.OnWiki, lookup.TestLookupMessage("/wiki", groupID, userID, telego.ChatTypeSupergroup))
	if got := len(caller.MethodCalls("sendMessage")) - beforeGroup; got != 2 {
		t.Fatalf("guarded-group lookup sends = %d, want 2 rate-limit-exempt replies", got)
	}
	groupReplies := caller.MethodCalls("sendMessage")[beforeGroup:]
	var firstGroup telego.SendMessageParams
	if err := json.Unmarshal(groupReplies[0].Body, &firstGroup); err != nil {
		t.Fatal(err)
	}
	if firstGroup.Text != i18n.Messages.LookupContent.Wiki.Usage.For(i18n.LangEN) {
		t.Fatalf("group answer = %q, want catalogue usage", firstGroup.Text)
	}

}

func TestLookupHandlerScheduledCleanup(t *testing.T) {
	const (
		groupID = int64(-1009999900013)
		userID  = int64(88)
	)
	ttl := 1
	cleanupCaller := &lookup.TestLookupTelegramCaller{Deletes: make(chan telego.DeleteMessageParams, 2)}
	cleanupBot := lookup.TestNewLookupTestBot(t, cleanupCaller)
	cleanupService := newLookupTestService(nil, telegram.NewConnector(cleanupBot), &settings.Config{
		Groups:           []settings.GroupConfig{{ID: groupID, Lang: "en"}},
		GroupIDs:         []int64{groupID},
		LookupTTLSeconds: &ttl,
	}, "")
	lookup.TestRunLookupHandler(t, cleanupBot, cleanupService.OnWiki, lookup.TestLookupMessage("/wiki", groupID, userID, telego.ChatTypeSupergroup))

	var deletes []telego.DeleteMessageParams
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	for len(deletes) < 2 {
		select {
		case params := <-cleanupCaller.Deletes:
			deletes = append(deletes, params)
		case <-deadline.C:
			t.Fatalf("scheduled cleanup calls = %+v, want response and command deletion", deletes)
		}
	}
	wantDeleteIDs := []int{101, 41}
	gotDeleteIDs := []int{deletes[0].MessageID, deletes[1].MessageID}
	if !reflect.DeepEqual(gotDeleteIDs, wantDeleteIDs) {
		t.Fatalf("scheduled cleanup message IDs = %v, want response then command %v", gotDeleteIDs, wantDeleteIDs)
	}
	for _, deletion := range deletes {
		if deletion.ChatID.ID != groupID {
			t.Fatalf("scheduled cleanup chat = %d, want %d", deletion.ChatID.ID, groupID)
		}
	}
}
