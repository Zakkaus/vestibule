package api

import (
	"net/http"
	"strings"

	"github.com/Zakkaus/vestibule/web"
)

func (s *Server) fonts(writer http.ResponseWriter, request *http.Request) {
	if !strings.HasSuffix(request.URL.Path, ".woff2") && request.URL.Path != "/fonts/OFL.txt" {
		s.writeNotFound(writer, request)
		return
	}
	writer.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	web.FontHandler.ServeHTTP(writer, request)
}
