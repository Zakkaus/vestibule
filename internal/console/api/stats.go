package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/Zakkaus/vestibule/internal/console/auth"
	"github.com/Zakkaus/vestibule/internal/verification"
)

type statsRangeResponse struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Timezone string `json:"timezone"`
}

type statsOutcomeResponse struct {
	Challenges int64   `json:"challenges"`
	Approved   int64   `json:"approved"`
	Declined   int64   `json:"declined"`
	Banned     int64   `json:"banned"`
	Expired    int64   `json:"expired"`
	PassRate   float64 `json:"pass_rate"`
}

type statsDayResponse struct {
	Date string `json:"date"`
	statsOutcomeResponse
}

type statsInterceptionResponse struct {
	Kind  string `json:"kind"`
	Count int64  `json:"count"`
}

type statsResponse struct {
	Range         statsRangeResponse          `json:"range"`
	Summary       statsOutcomeResponse        `json:"summary"`
	Trend         []statsDayResponse          `json:"trend"`
	Interceptions []statsInterceptionResponse `json:"interceptions"`
}

func (s *Server) statsRoute(
	writer http.ResponseWriter,
	request *http.Request,
	chatID int64,
	rest []string,
) {
	if request.Method == http.MethodGet && len(rest) == 0 {
		s.stats(writer, request, chatID)
		return
	}
	writeError(writer, http.StatusNotFound, "not_found")
}

func (s *Server) stats(writer http.ResponseWriter, request *http.Request, chatID int64) {
	if _, ok := s.authorizedSession(writer, request, chatID, auth.ReadAccess); !ok {
		return
	}
	statsRequest, err := parseStatsRequest(request, chatID)
	if err != nil {
		writeError(writer, http.StatusBadRequest, "invalid_stats_query")
		return
	}
	report, err := s.verification.ConsoleStats(request.Context(), statsRequest)
	if err != nil {
		if errors.Is(err, verification.ErrConsoleStatsInvalid) {
			writeError(writer, http.StatusBadRequest, "invalid_stats_query")
			return
		}
		writeError(writer, http.StatusServiceUnavailable, "stats_unavailable")
		return
	}
	writeJSON(writer, http.StatusOK, statsView(report))
}

func parseStatsRequest(request *http.Request, chatID int64) (verification.ConsoleStatsRequest, error) {
	query := request.URL.Query()
	fromText, fromOK := oneQueryValue(query["from"])
	toText, toOK := oneQueryValue(query["to"])
	timezone, timezoneOK := oneQueryValue(query["timezone"])
	if !fromOK || !toOK || !timezoneOK || len(timezone) > 255 {
		return verification.ConsoleStatsRequest{}, verification.ErrConsoleStatsInvalid
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return verification.ConsoleStatsRequest{}, verification.ErrConsoleStatsInvalid
	}
	from, err := time.ParseInLocation(time.DateOnly, fromText, location)
	if err != nil {
		return verification.ConsoleStatsRequest{}, verification.ErrConsoleStatsInvalid
	}
	to, err := time.ParseInLocation(time.DateOnly, toText, location)
	if err != nil {
		return verification.ConsoleStatsRequest{}, verification.ErrConsoleStatsInvalid
	}
	return verification.ConsoleStatsRequest{
		GroupID: chatID, From: from, To: to, Location: location,
	}, nil
}

func oneQueryValue(values []string) (string, bool) {
	returnValue := ""
	if len(values) == 1 {
		returnValue = values[0]
	}
	return returnValue, len(values) == 1 && returnValue != ""
}

func statsView(report verification.ConsoleStatsReport) statsResponse {
	response := statsResponse{
		Range:         statsRangeResponse{From: report.From, To: report.To, Timezone: report.Timezone},
		Summary:       statsOutcomeView(report.Summary),
		Trend:         make([]statsDayResponse, 0, len(report.Trend)),
		Interceptions: make([]statsInterceptionResponse, 0, len(report.Interceptions)),
	}
	for _, day := range report.Trend {
		response.Trend = append(response.Trend, statsDayResponse{
			Date: day.Date, statsOutcomeResponse: statsOutcomeView(day.Outcome),
		})
	}
	for _, interception := range report.Interceptions {
		response.Interceptions = append(response.Interceptions, statsInterceptionResponse{
			Kind: interception.Kind, Count: interception.Count,
		})
	}
	return response
}

func statsOutcomeView(outcome verification.ConsoleStatsOutcome) statsOutcomeResponse {
	return statsOutcomeResponse{
		Challenges: outcome.Challenges,
		Approved:   outcome.Approved,
		Declined:   outcome.Declined,
		Banned:     outcome.Banned,
		Expired:    outcome.Expired,
		PassRate:   outcome.PassRate,
	}
}
