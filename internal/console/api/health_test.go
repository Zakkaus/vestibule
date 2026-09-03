package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Zakkaus/vestibule/internal/status"
)

func TestReadinessEndpointRefusesTrafficUntilEveryDependencyIsReady(t *testing.T) {
	healthyDatabase := func(context.Context) error { return nil }
	tests := []struct {
		name          string
		harm          string
		configReady   bool
		telegramReady bool
		database      func(context.Context) error
		wantStatus    int
	}{
		{
			name:          "configuration incomplete",
			harm:          "readiness accepted traffic before configuration completed",
			telegramReady: true,
			database:      healthyDatabase,
			wantStatus:    http.StatusServiceUnavailable,
		},
		{
			name:        "Telegram channel unavailable",
			harm:        "readiness accepted traffic without a Telegram channel",
			configReady: true,
			database:    healthyDatabase,
			wantStatus:  http.StatusServiceUnavailable,
		},
		{
			name:          "database probe unavailable",
			harm:          "readiness accepted traffic without a database probe",
			configReady:   true,
			telegramReady: true,
			wantStatus:    http.StatusServiceUnavailable,
		},
		{
			name:          "database unhealthy",
			harm:          "readiness accepted traffic while the database was unhealthy",
			configReady:   true,
			telegramReady: true,
			database: func(context.Context) error {
				return errors.New("database unavailable")
			},
			wantStatus: http.StatusServiceUnavailable,
		},
		{
			name:          "all dependencies ready",
			configReady:   true,
			telegramReady: true,
			database:      healthyDatabase,
			wantStatus:    http.StatusOK,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			health := status.NewHealth(test.database)
			health.SetConfigReady(test.configReady)
			health.SetTelegramReady(test.telegramReady)

			response := getPath(New(Config{Health: health}), "/readyz")
			if response.Code != test.wantStatus {
				t.Fatalf("%s: GET /readyz = %d, want %d", test.harm, response.Code, test.wantStatus)
			}
		})
	}
}

func TestReadinessEndpointBoundsDatabaseProbe(t *testing.T) {
	var hasDeadline bool
	unhealthy := status.NewHealth(func(ctx context.Context) error {
		_, hasDeadline = ctx.Deadline()
		return errors.New("database unavailable")
	})
	unhealthy.SetConfigReady(true)
	unhealthy.SetTelegramReady(true)

	if response := getPath(New(Config{Health: unhealthy}), "/readyz"); response.Code != http.StatusServiceUnavailable {
		t.Fatalf("GET /readyz with an unhealthy database = %d, want 503", response.Code)
	}
	if !hasDeadline {
		t.Fatal("readiness database probe had no deadline; a stuck database could wedge the endpoint")
	}

	healthy := status.NewHealth(func(context.Context) error { return nil })
	healthy.SetConfigReady(true)
	healthy.SetTelegramReady(true)
	if response := getPath(New(Config{Health: healthy}), "/readyz"); response.Code != http.StatusOK {
		t.Fatalf("GET /readyz with healthy dependencies = %d, want 200", response.Code)
	}
}

func TestHealthEndpointsRejectNonGETRequests(t *testing.T) {
	health := status.NewHealth(func(context.Context) error { return nil })
	health.SetConfigReady(true)
	health.SetTelegramReady(true)
	server := New(Config{Health: health})

	for _, path := range []string{"/livez", "/readyz"} {
		if response := getPath(server, path); response.Code != http.StatusOK {
			t.Fatalf("GET %s = %d, want 200", path, response.Code)
		}
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodPost, path, nil))
		if response.Code != http.StatusNotFound || decodeError(response) != "not_found" {
			t.Fatalf("POST %s = %d code=%q; a non-GET request must not impersonate a health probe",
				path, response.Code, decodeError(response))
		}
	}
}

func TestStoppingAdmissionMakesReadinessUnavailable(t *testing.T) {
	health := status.NewHealth(func(context.Context) error { return nil })
	health.SetConfigReady(true)
	health.SetTelegramReady(true)
	server := New(Config{Health: health})

	if response := getPath(server, "/readyz"); response.Code != http.StatusOK {
		t.Fatalf("GET /readyz before stopping admission = %d, want 200", response.Code)
	}
	if err := server.StopAdmission(); err != nil {
		t.Fatalf("stop console admission: %v", err)
	}
	response := getPath(server, "/readyz")
	if response.Code != http.StatusServiceUnavailable || decodeError(response) != "not_ready" {
		t.Fatalf("GET /readyz after stopping admission = %d code=%q; traffic could reach a shutting-down process",
			response.Code, decodeError(response))
	}
}
