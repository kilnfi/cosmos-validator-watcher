package app

import (
	"context"
	"fmt"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Probe func() bool

var upProbe Probe = func() bool { return true }

type HTTPServer struct {
	server *http.Server
}

func NewHTTPServer(addr string, readyFn Probe) *HTTPServer {
	s := &HTTPServer{
		server: &http.Server{Addr: addr},
	}

	http.Handle("/metrics", promhttp.Handler())
	http.HandleFunc("/ready", probeHandler(readyFn))
	http.HandleFunc("/live", probeHandler(upProbe))

	return s
}

func (s *HTTPServer) Run() error {
	return s.listenAndServe()
}

func (s *HTTPServer) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

func (s *HTTPServer) listenAndServe() error {
	err := s.server.ListenAndServe()

	if err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("failed starting HTTP server: %w", err)
	}

	return nil
}

func probeHandler(probe Probe) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if probe() {
			w.WriteHeader(http.StatusNoContent)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}
}
