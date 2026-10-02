package lookup

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type bugTestRoundTripper func(*http.Request) (*http.Response, error)

func (f bugTestRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestFetchBugLookupState(t *testing.T) {
	oldClient := httpClient
	t.Cleanup(func() { httpClient = oldClient })

	tests := []struct {
		name      string
		status    int
		body      string
		err       error
		wantState BugLookupState
		wantTitle string
	}{
		{
			name:      "found",
			status:    http.StatusOK,
			body:      `{"bugs":[{"summary":"Example","status":"CONFIRMED"}]}`,
			wantState: BugLookupFound,
			wantTitle: "Example",
		},
		{name: "genuine 404", status: http.StatusNotFound, wantState: BugLookupNotFound},
		{name: "rate limited", status: http.StatusTooManyRequests, wantState: bugLookupUnavailable},
		{name: "server failure", status: http.StatusInternalServerError, wantState: bugLookupUnavailable},
		{name: "timeout", err: context.DeadlineExceeded, wantState: bugLookupUnavailable},
		{name: "empty 200", status: http.StatusOK, body: `{"bugs":[]}`, wantState: bugLookupUnavailable},
		{name: "error 200", status: http.StatusOK, body: `{"error":true}`, wantState: bugLookupUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpClient = &http.Client{Transport: bugTestRoundTripper(func(*http.Request) (*http.Response, error) {
				if tt.err != nil {
					return nil, tt.err
				}
				return &http.Response{
					StatusCode: tt.status,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(tt.body)),
				}, nil
			})}
			info, state := FetchBug(context.Background(), "123")
			if state != tt.wantState {
				t.Errorf("fetchBug() state = %v, want %v", state, tt.wantState)
			}
			if info.Summary != tt.wantTitle {
				t.Errorf("fetchBug() summary = %q, want %q", info.Summary, tt.wantTitle)
			}
		})
	}
}
