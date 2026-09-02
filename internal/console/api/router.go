package api

import "net/http"

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
		authenticator:   config.Authenticator,
		verification:    config.Verification,
		settings:        config.Settings,
		rules:           config.Rules,
		processSettings: config.ProcessSettings,
		health:          config.Health,
		persistence:     config.Persistence,
		replacement:     config.Replacement,
		release:         config.Release,
		version:         config.Version,
		setup:           config.Setup,
		setupClaimed:    config.SetupClaimed,
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
	routes.server.serveHTTP(writer, request)
}
