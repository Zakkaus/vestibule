package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/Zakkaus/vestibule/internal/console/auth"
	"github.com/Zakkaus/vestibule/internal/settings"
)

type feedsResponse struct {
	Revision    uint64                                     `json:"revision"`
	Feed        feedsView                                  `json:"feed"`
	GitHubRepos settingResponse[[]feedsGitHubRepoResponse] `json:"github_repos"`
}

type feedsGitHubRepoResponse struct {
	Repo   string `json:"repo"`
	Branch string `json:"branch"`
	Issues bool   `json:"issues"`
	Pulls  bool   `json:"pulls"`
}

type feedsGitHubRepoRequest struct {
	Repo   string `json:"repo"`
	Branch string `json:"branch"`
	Issues *bool  `json:"issues"`
	Pulls  *bool  `json:"pulls"`
}

type feedsView struct {
	Lang            settingResponse[string] `json:"lang"`
	IntervalSeconds settingResponse[int]    `json:"interval_seconds"`
	Bugs            settingResponse[bool]   `json:"bugs"`
	News            settingResponse[bool]   `json:"news"`
	BugProduct      settingResponse[string] `json:"bug_product"`
	BugComponent    settingResponse[string] `json:"bug_component"`
	SilentBugs      settingResponse[bool]   `json:"silent_bugs"`
}

func feedsViewForGroup(group settings.GroupView) feedsResponse {
	feed := group.Feed()
	return feedsResponse{
		Revision:    group.Revision(),
		GitHubRepos: feedsGitHubReposView(feed.GitHubRepos),
		Feed: feedsView{
			Lang: settingView(feed.Lang), IntervalSeconds: settingView(feed.IntervalSeconds),
			Bugs: settingView(feed.Bugs), News: settingView(feed.News),
			BugProduct: settingView(feed.BugProduct), BugComponent: settingView(feed.BugComponent),
			SilentBugs: settingView(feed.SilentBugs),
		},
	}
}

func feedsGitHubReposView(value settings.Setting[[]settings.GitHubRepo]) settingResponse[[]feedsGitHubRepoResponse] {
	repositories := make([]feedsGitHubRepoResponse, len(value.Value))
	for i, repository := range value.Value {
		repositories[i] = feedsGitHubRepoResponse{
			Repo: repository.Repo, Branch: repository.Branch,
			Issues: repository.IssuesOn(), Pulls: repository.PullsOn(),
		}
	}
	return settingResponse[[]feedsGitHubRepoResponse]{Value: repositories, Source: value.Source.String()}
}

type feedsUpdateRequest struct {
	ExpectedRevision *json.Number              `json:"expected_revision"`
	Lang             *string                   `json:"lang"`
	IntervalSeconds  *int                      `json:"interval_seconds"`
	Bugs             *bool                     `json:"bugs"`
	News             *bool                     `json:"news"`
	BugProduct       *string                   `json:"bug_product"`
	BugComponent     *string                   `json:"bug_component"`
	SilentBugs       *bool                     `json:"silent_bugs"`
	GitHubRepos      *[]feedsGitHubRepoRequest `json:"github_repos"`
}

func (s *Server) feedsRoute(writer http.ResponseWriter, request *http.Request, chatID int64, rest []string) {
	if len(rest) != 0 {
		writeError(writer, http.StatusNotFound, "not_found")
		return
	}
	switch request.Method {
	case http.MethodGet:
		s.readFeeds(writer, request, chatID)
	case http.MethodPut:
		s.putFeeds(writer, request, chatID)
	default:
		writeError(writer, http.StatusNotFound, "not_found")
	}
}

func (s *Server) readFeeds(writer http.ResponseWriter, request *http.Request, chatID int64) {
	if _, ok := s.authorizedSession(writer, request, chatID, auth.ReadAccess); !ok {
		return
	}
	group, ok := s.settingsGroup(writer, chatID)
	if !ok {
		return
	}
	writeJSON(writer, http.StatusOK, feedsViewForGroup(group))
}

func (s *Server) putFeeds(writer http.ResponseWriter, request *http.Request, chatID int64) {
	session, ok := s.authorizedSession(writer, request, chatID, auth.WriteAccess)
	if !ok {
		return
	}
	if err := s.authenticator.ValidateCSRF(request, session); err != nil {
		writeError(writer, http.StatusForbidden, "csrf_invalid")
		return
	}
	group, ok := s.settingsGroup(writer, chatID)
	if !ok {
		return
	}
	var input feedsUpdateRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	if input.ExpectedRevision == nil {
		writeFeedsInvalid(writer, []*settings.FeedValidationError{{Field: "expected_revision", Code: "invalid_revision"}})
		return
	}
	expected, err := strconv.ParseUint(input.ExpectedRevision.String(), 10, 64)
	if err != nil || expected > maxSafeJSONInteger {
		writeFeedsInvalid(writer, []*settings.FeedValidationError{{Field: "expected_revision", Code: "invalid_revision"}})
		return
	}
	if fields := missingFeedsUpdateFields(input); len(fields) > 0 {
		writeFeedsInvalid(writer, fields)
		return
	}
	repositories := make([]settings.GitHubRepo, len(*input.GitHubRepos))
	for i, repository := range *input.GitHubRepos {
		repositories[i] = settings.GitHubRepo{
			Repo: repository.Repo, Branch: repository.Branch,
			Issues: repository.Issues, Pulls: repository.Pulls,
		}
	}
	next := group.Overrides()
	next.Feed = &settings.FeedOverride{
		Lang: input.Lang, IntervalSeconds: input.IntervalSeconds, Bugs: input.Bugs, News: input.News,
		BugProduct: input.BugProduct, BugComponent: input.BugComponent, SilentBugs: input.SilentBugs,
		GitHubRepos: &repositories,
	}
	if _, err := s.settings.Update(chatID, expected, next); err != nil {
		writeFeedsError(writer, err)
		return
	}
	group, ok = s.settingsGroup(writer, chatID)
	if !ok {
		return
	}
	writeJSON(writer, http.StatusOK, feedsViewForGroup(group))
}

func missingFeedsUpdateFields(input feedsUpdateRequest) []*settings.FeedValidationError {
	fields := make([]*settings.FeedValidationError, 0, 8)
	appendMissing := func(missing bool, name string) {
		if missing {
			fields = append(fields, &settings.FeedValidationError{Field: name, Code: "required_field"})
		}
	}
	appendMissing(input.Lang == nil, "lang")
	appendMissing(input.IntervalSeconds == nil, "interval_seconds")
	appendMissing(input.Bugs == nil, "bugs")
	appendMissing(input.News == nil, "news")
	appendMissing(input.BugProduct == nil, "bug_product")
	appendMissing(input.BugComponent == nil, "bug_component")
	appendMissing(input.SilentBugs == nil, "silent_bugs")
	appendMissing(input.GitHubRepos == nil, "github_repos")
	if input.GitHubRepos != nil {
		for i, repository := range *input.GitHubRepos {
			appendMissing(repository.Issues == nil, fmt.Sprintf("github_repos[%d].issues", i))
			appendMissing(repository.Pulls == nil, fmt.Sprintf("github_repos[%d].pulls", i))
		}
	}
	return fields
}

type feedsFieldError struct {
	Name string `json:"name"`
	Code string `json:"code"`
}

type feedsInvalidResponse struct {
	Error struct {
		Code string `json:"code"`
	} `json:"error"`
	Fields []feedsFieldError `json:"fields,omitempty"`
}

func writeFeedsInvalid(writer http.ResponseWriter, fields []*settings.FeedValidationError) {
	response := feedsInvalidResponse{}
	response.Error.Code = "invalid_request"
	for _, field := range fields {
		if field != nil {
			response.Fields = append(response.Fields, feedsFieldError{Name: field.Field, Code: field.Code})
		}
	}
	writeJSON(writer, http.StatusBadRequest, response)
}

func writeFeedsError(writer http.ResponseWriter, err error) {
	var validation *settings.FeedValidationError
	switch {
	case errors.As(err, &validation):
		writeFeedsInvalid(writer, []*settings.FeedValidationError{validation})
	case errors.Is(err, settings.ErrSettingsConflict):
		writeError(writer, http.StatusConflict, "settings_conflict")
	case errors.Is(err, settings.ErrUnknownGroup):
		writeError(writer, http.StatusNotFound, "chat_not_found")
	default:
		writeError(writer, http.StatusServiceUnavailable, "settings_unavailable")
	}
}
