package feed

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/lookup"
	"github.com/Zakkaus/vestibule/internal/settings"
	"github.com/mymmrac/telego"
	ta "github.com/mymmrac/telego/telegoapi"
)

// A separate process isolates every lookup global changed by New without adding a production test API.
func TestGitHubPrimaryDelivery(t *testing.T) {
	if os.Getenv("GHFEED_PRIMARY_CHILD") == "1" {
		primaryGitHubDelivery(t)
		return
	}
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(binary, "-test.run=^TestGitHubPrimaryDelivery$", "-test.v")
	cmd.Env = append(os.Environ(), "GHFEED_PRIMARY_CHILD=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("isolated GitHub delivery failed: %v\n%s", err, out)
	}
	t.Logf("%s", out)
}

type primaryTransport func(*http.Request) (*http.Response, error)

func (f primaryTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type primaryCaller struct {
	bodies []json.RawMessage
	cancel context.CancelFunc
	sentAt []time.Time
}

func (c *primaryCaller) Call(_ context.Context, endpoint string, data *ta.RequestData) (*ta.Response, error) {
	if !strings.HasSuffix(endpoint, "/sendMessage") {
		return nil, fmt.Errorf("unexpected Telegram method %s", endpoint)
	}
	c.bodies = append(c.bodies, append(json.RawMessage(nil), data.BodyRaw...))
	c.sentAt = append(c.sentAt, time.Now())
	if c.cancel != nil {
		c.cancel()
	}
	body, err := json.Marshal(telego.Message{MessageID: 100 + len(c.bodies)})
	return &ta.Response{Ok: true, Result: body}, err
}

func primaryGitHubFixture(t *testing.T) ([]byte, string) {
	t.Helper()
	body, err := os.ReadFile("../../testdata/upstream/github/commits-vestibule.atom")
	if err != nil {
		t.Fatal(err)
	}
	var page struct {
		Entries []struct {
			ID string `xml:"id"`
		} `xml:"entry"`
	}
	if err := xml.Unmarshal(body, &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Entries) != 20 {
		t.Fatalf("fixture entries = %d, want 20", len(page.Entries))
	}
	return body, page.Entries[0].ID
}

func primaryGitHubDelivery(t *testing.T) {
	fixture, baseline := primaryGitHubFixture(t)
	var hits atomic.Int32
	var added atomic.Value
	added.Store("")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() != "/mount&amp;copy/Zakkaus/vestibule/commits.atom" {
			http.NotFound(w, r)
			return
		}
		hits.Add(1)
		body := strings.Replace(string(fixture), "<entry>", added.Load().(string)+"<entry>", 1)
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()
	primaryRestrictHTTP(t, server.URL)
	config := primaryLoadConfig(t, server.URL+"/mount&amp;copy")
	lookup.New(nil, nil, config, "")
	feedSendPause = 0
	caller := &primaryCaller{}
	bot := newAPITestBot(t, caller)
	f := &config.Feeds[0]
	states := map[int64]*feedState{f.ChatID: {}}
	dir := t.TempDir()
	next := map[int64]time.Time{}
	now := time.Unix(1000000, 0)
	pollAll(context.Background(), bot, []*settings.FeedConfig{f}, states, dir, now, next)
	if hits.Load() != 1 || len(caller.bodies) != 0 {
		t.Fatalf("first round: Atom requests=%d, Telegram requests=%d; want 1 and 0", hits.Load(), len(caller.bodies))
	}
	primaryAssertCursor(t, dir, f.ChatID, baseline)
	added.Store(primaryAddedEntries("c", "b", "a"))
	pollAll(context.Background(), bot, []*settings.FeedConfig{f}, states, dir, now.Add(f.Interval()), next)
	if hits.Load() != 2 || len(caller.bodies) != 3 {
		t.Fatalf("second round: Atom requests=%d, Telegram requests=%d; want 2 and 3", hits.Load(), len(caller.bodies))
	}
	primaryAssertMessages(t, caller.bodies, f.ChatID, server.URL)
	primaryAssertCursor(t, dir, f.ChatID, "tag:github.com,2008:Grit::Commit/"+strings.Repeat("c", 40))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	caller.cancel = cancel
	added.Store(primaryAddedEntries("e", "d", "c", "b", "a"))
	pollAll(ctx, bot, []*settings.FeedConfig{f}, states, dir, now.Add(2*f.Interval()), next)
	if len(caller.bodies) != 4 {
		t.Fatalf("canceled round sent %d messages in total, want 4", len(caller.bodies))
	}
	primaryAssertCursor(t, dir, f.ChatID, "tag:github.com,2008:Grit::Commit/"+strings.Repeat("d", 40))
}

func primaryRestrictHTTP(t *testing.T, address string) {
	t.Helper()
	allowed, err := url.Parse(address)
	if err != nil {
		t.Fatal(err)
	}
	transport := http.DefaultTransport
	http.DefaultTransport = primaryTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != allowed.Host {
			return nil, fmt.Errorf("test refuses external HTTP destination %s", r.URL.Host)
		}
		return transport.RoundTrip(r)
	})
	t.Cleanup(func() { http.DefaultTransport = transport })
}

func primaryLoadConfig(t *testing.T, base string) *settings.Config {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	body := fmt.Sprintf(`{"github_atom_base":%q,"feeds":[{"chat_id":-1009000002401,"lang":"en","interval_seconds":60,"bugs":false,"news":false,"silent_bugs":true,"github_repos":[{"repo":"Zakkaus/vestibule"}]}]}`, base)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	config, err := settings.LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	return config
}

func primaryAddedEntries(shas ...string) string {
	var body strings.Builder
	for _, sha := range shas {
		fmt.Fprintf(&body, `<entry><id>tag:github.com,2008:Grit::Commit/%s</id><title type="text">Commit %s &amp;lt;tag&amp;gt;</title><author><name>Author &amp;amp;</name></author></entry>`, strings.Repeat(sha, 40), sha)
	}
	return body.String()
}

func primaryAssertCursor(t *testing.T, dir string, chat int64, want string) {
	t.Helper()
	body, err := os.ReadFile(feedStatePath(dir, chat))
	if err != nil {
		t.Fatal(err)
	}
	var state struct {
		GitHub struct {
			Repos map[string]struct {
				LastID string `json:"last_id"`
			} `json:"repos"`
		} `json:"github"`
	}
	if err := json.Unmarshal(body, &state); err != nil {
		t.Fatal(err)
	}
	if got := state.GitHub.Repos["Zakkaus/vestibule@"].LastID; got != want {
		t.Fatalf("persisted cursor = %q, want %q; JSON=%s", got, want, body)
	}
}

func primaryAssertMessages(t *testing.T, bodies []json.RawMessage, chat int64, base string) {
	t.Helper()
	for i, raw := range bodies {
		var wire struct {
			ChatID              int64  `json:"chat_id"`
			Text                string `json:"text"`
			ParseMode           string `json:"parse_mode"`
			DisableNotification bool   `json:"disable_notification"`
			Preview             struct {
				Disabled bool `json:"is_disabled"`
			} `json:"link_preview_options"`
		}
		if err := json.Unmarshal(raw, &wire); err != nil {
			t.Fatal(err)
		}
		sha := string(rune('a' + i))
		want := fmt.Sprintf(`<a href="%s/mount&amp;amp;copy/Zakkaus/vestibule/commit/%s"><b>Zakkaus/vestibule</b></a> · <code>%s</code>`+"\nCommit %s &amp;lt;tag&amp;gt;\n<b>Author</b>: Author &amp;amp;", base, strings.Repeat(sha, 40), strings.Repeat(sha, 7), sha)
		if wire.Text != want || wire.ChatID != chat || wire.ParseMode != "HTML" || wire.DisableNotification || !wire.Preview.Disabled {
			t.Fatalf("message %d violates ordered rendering/notification/preview contract: %s; want text=%q", i, raw, want)
		}
	}
}
