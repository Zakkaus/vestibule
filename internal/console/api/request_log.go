package api

import (
	"net/http"
	"strings"
	"time"
)

type loggedResponse struct {
	http.ResponseWriter
	status, size int
}

func (w *loggedResponse) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
func (w *loggedResponse) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(body)
	w.size += n
	return n, err
}
func redactedRequestPath(path string) string {
	for _, prefix := range []string{"/verify/", "/enter/", "/setup/"} {
		if index := strings.Index(path, prefix); index >= 0 {
			return path[:index] + prefix + "[redacted]"
		}
	}
	return path
}
func (s *Server) logRequest(request *http.Request, response *loggedResponse, elapsed time.Duration) {
	status := response.status
	if status == 0 {
		status = http.StatusOK
	}
	if status < 300 && (request.URL.Path == "/livez" || request.URL.Path == "/readyz") {
		return
	}
	s.requestLog.Printf("http method=%s path=%q status=%d bytes=%d elapsed=%s", request.Method, redactedRequestPath(request.URL.Path), status, response.size, elapsed)
}
