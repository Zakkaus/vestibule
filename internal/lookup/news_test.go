package lookup

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Zakkaus/vestibule/internal/i18n"
)

type newsLookupResult struct {
	items     []NewsItem
	available bool
}

func expectFreshNewsLookup(t *testing.T, results <-chan newsLookupResult) {
	t.Helper()
	select {
	case got := <-results:
		if !got.available || len(got.items) != 1 || got.items[0].Title != "Refreshed news" {
			t.Errorf("the initial valid news lookup did not complete with fresh items: %+v, available=%v", got.items, got.available)
		}
	case <-time.After(time.Second):
		t.Error("the initial news lookup did not finish after the fetch was released")
	}
}

func TestGetNewsAvailability(t *testing.T) {
	newsC.mu.Lock()
	oldItems, oldFetched, oldLoading := newsC.items, newsC.fetched, newsC.loading
	newsC.mu.Unlock()
	oldURL := newsURL
	t.Cleanup(func() {
		newsC.mu.Lock()
		newsC.items, newsC.fetched, newsC.loading = oldItems, oldFetched, oldLoading
		newsC.mu.Unlock()
		newsURL = oldURL
	})

	tests := []struct {
		name          string
		status        int
		body          string
		seed          []NewsItem
		wantAvailable bool
		wantItems     int
	}{
		{
			name:          "empty index answered",
			status:        http.StatusOK,
			wantAvailable: true,
		},
		{
			name:          "index answered with news",
			status:        http.StatusOK,
			body:          `<a href="/support/news-items/2026-08-24-test.html">Test news<`,
			wantAvailable: true,
			wantItems:     1,
		},
		{
			name:      "HTTP failure with stale data",
			status:    http.StatusServiceUnavailable,
			seed:      []NewsItem{{Date: "2026-08-23", Title: "Cached", URL: "https://example.test/cached"}},
			wantItems: 1,
		},
		{
			name:   "markup drift",
			status: http.StatusOK,
			body:   `<html><body>layout changed</body></html>`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			newsURL = srv.URL
			newsC.mu.Lock()
			newsC.items, newsC.fetched, newsC.loading = tt.seed, time.Time{}, false
			newsC.mu.Unlock()

			items, available := GetNews(context.Background())
			if available != tt.wantAvailable {
				t.Errorf("GetNews() availability = %v, want %v", available, tt.wantAvailable)
			}
			if len(items) != tt.wantItems {
				t.Errorf("GetNews() returned %d items, want %d", len(items), tt.wantItems)
			}
		})
	}
}

func TestConcurrentNewsLookupReturnsPreviousItemsWithoutSecondFetch(t *testing.T) {
	newsC.mu.Lock()
	oldItems, oldFetched, oldLoading := newsC.items, newsC.fetched, newsC.loading
	previous := []NewsItem{{Date: "2026-08-23", Title: "Cached news", URL: "https://example.test/cached"}}
	newsC.items, newsC.fetched, newsC.loading = previous, time.Time{}, false
	newsC.mu.Unlock()
	oldURL := newsURL
	t.Cleanup(func() {
		newsC.mu.Lock()
		newsC.items, newsC.fetched, newsC.loading = oldItems, oldFetched, oldLoading
		newsC.mu.Unlock()
		newsURL = oldURL
	})
	started := make(chan struct{})
	release := make(chan struct{})
	firstDone := make(chan struct{})
	released := false
	releaseFirst := func() {
		if !released {
			close(release)
			released = true
		}
	}
	var fetches atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if fetches.Add(1) == 1 {
			close(started)
			<-release
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<a href="/support/news-items/2026-08-24-refreshed.html">Refreshed news<`))
	}))
	t.Cleanup(func() {
		releaseFirst()
		select {
		case <-firstDone:
		case <-time.After(time.Second):
			t.Error("the first news refresh did not finish")
		}
		srv.Close()
	})
	newsURL = srv.URL
	firstResult := make(chan newsLookupResult, 1)
	go func() {
		items, available := GetNews(context.Background())
		firstResult <- newsLookupResult{items: items, available: available}
		close(firstDone)
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("the first news lookup did not begin its fetch")
	}

	items, available := GetNews(context.Background())
	if available {
		t.Error("a concurrent lookup reported a stale cache as freshly fetched")
	}
	if len(items) != 1 || items[0].Title != previous[0].Title {
		t.Errorf("a concurrent lookup did not return the previous news items: got %+v, want %+v", items, previous)
	}
	if got := fetches.Load(); got != 1 {
		t.Fatalf("a concurrent news lookup started a second upstream fetch: got %d fetches, want 1", got)
	}

	releaseFirst()
	expectFreshNewsLookup(t, firstResult)
}

func TestRenderNewsAvailability(t *testing.T) {
	item := NewsItem{Date: "2026-08-24", Title: "Kernel update", URL: "https://example.test/kernel"}
	tests := []struct {
		name      string
		arg       string
		items     []NewsItem
		available bool
		want      []string
		notWant   string
	}{
		{
			name:      "authoritative empty result",
			arg:       "missing",
			available: true,
			want:      []string{i18n.Messages.LookupContent.News.NoMatches.For(i18n.LangZH)},
			notWant:   i18n.Messages.LookupContent.News.Unavailable.For(i18n.LangZH),
		},
		{
			name:    "index unavailable",
			arg:     "missing",
			want:    []string{i18n.Messages.LookupContent.News.Unavailable.For(i18n.LangZH)},
			notWant: i18n.Messages.LookupContent.News.NoMatches.For(i18n.LangZH),
		},
		{
			name:      "available filtered miss",
			arg:       "missing",
			items:     []NewsItem{item},
			available: true,
			want:      []string{i18n.Messages.LookupContent.News.NoMatches.For(i18n.LangZH)},
		},
		{
			name:    "stale hit is incomplete",
			arg:     "kernel",
			items:   []NewsItem{item},
			want:    []string{item.Title, i18n.Messages.LookupContent.News.Stale.For(i18n.LangZH)},
			notWant: i18n.Messages.LookupContent.News.NoMatches.For(i18n.LangZH),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := renderNews(i18n.LangZH, tt.arg, tt.items, tt.available)
			for _, want := range tt.want {
				if !strings.Contains(got, want) {
					t.Errorf("renderNews() = %q, want substring %q", got, want)
				}
			}
			if tt.notWant != "" && strings.Contains(got, tt.notWant) {
				t.Errorf("renderNews() = %q, unwanted substring %q", got, tt.notWant)
			}
		})
	}
}
