package api

import (
	"context"
	"log"
	"net/http"
	"time"
)

type routeSet struct {
	server *Server
}

func New(config Config) *Server {
	server := &Server{health: config.Health}
	server.ReplaceRoutes(config)
	return server
}

// ReplaceRoutes atomically installs a complete new route table for the running listener.
func (s *Server) ReplaceRoutes(config Config) {
	routes := &Server{
		authenticator:              config.Authenticator,
		verification:               config.Verification,
		settings:                   config.Settings,
		chatTitleResolver:          config.ChatTitleResolver,
		chatAdministratorsResolver: config.ChatAdministratorsResolver,
		rules:                      config.Rules,
		processSettings:            config.ProcessSettings,
		health:                     config.Health,
		persistence:                config.Persistence,
		rollbackObservations:       config.RollbackObservations,
		rollbackRejections:         config.RollbackRejections,
		replacement:                config.Replacement,
		release:                    config.Release,
		daily:                      config.Daily,
		version:                    config.Version,
		observeOnly:                config.ObserveOnly,
		setup:                      config.Setup,
		setupClaimed:               config.SetupClaimed,
		botUsername:                config.BotUsername,
		webVerification:            config.WebVerification,
		consoleURL:                 config.ConsoleURL,
		turnstileSiteKey:           config.TurnstileSiteKey,
		webOperatorAlert:           config.WebOperatorAlert,
		requestLog:                 config.RequestLog,
	}
	if routes.requestLog == nil {
		routes.requestLog = log.Default()
	}
	s.routes.Store(&routeSet{server: routes})
}

// Handler returns the router for focused tests and for the production HTTP server.
func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(s.dispatch)
}

func (s *Server) dispatch(writer http.ResponseWriter, request *http.Request) {
	routes := s.routes.Load()
	if routes == nil || routes.server == nil {
		writeError(writer, http.StatusServiceUnavailable, "router_unavailable")
		return
	}
	response := &loggedResponse{ResponseWriter: writer}
	started := time.Now()
	routes.server.serveHTTP(response, request)
	routes.server.logRequest(request, response, time.Since(started))
}
func (s *Server) live(writer http.ResponseWriter) {
	if s.health != nil && s.health.Live() {
		writeJSON(writer, http.StatusOK, map[string]string{"status": "ok"})
		return
	}
	writeError(writer, http.StatusServiceUnavailable, "not_live")
}

func (s *Server) ready(writer http.ResponseWriter, request *http.Request) {
	if s.health == nil {
		writeError(writer, http.StatusServiceUnavailable, "not_ready")
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), healthProbeTimeout)
	defer cancel()
	if s.health.Ready(ctx) {
		writeJSON(writer, http.StatusOK, map[string]string{"status": "ok"})
		return
	}
	writeError(writer, http.StatusServiceUnavailable, "not_ready")
}
