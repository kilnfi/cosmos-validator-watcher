package app

import (
	"context"
	"fmt"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Probe func() bool

var upProbe Probe = func() bool { return true }

type HTTPServer struct {
	*http.Server
}

type HTTPMuxOption func(*http.ServeMux)

func withProbe(path string, probe Probe) HTTPMuxOption {
	return func(mux *http.ServeMux) {
		mux.HandleFunc(path, probeHandler(probe))
	}
}

func WithReadyProbe(probe Probe) HTTPMuxOption {
	return withProbe("/ready", probe)
}

func WithLiveProbe(probe Probe) HTTPMuxOption {
	return withProbe("/live", probe)
}

func WithMetrics(registry *prometheus.Registry) HTTPMuxOption {
	return func(mux *http.ServeMux) {
		mux.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
	}
}

func NewHTTPServer(addr string, options ...HTTPMuxOption) *HTTPServer {
	mux := http.NewServeMux()
	server := &HTTPServer{
		Server: &http.Server{
			Addr:    addr,
			Handler: mux,
		},
	}

	for _, option := range options {
		option(mux)
	}

	return server
}

func (s *HTTPServer) Run() error {
	return s.listenAndServe()
}

func (s *HTTPServer) Shutdown(ctx context.Context) error {
	return s.Server.Shutdown(ctx)
}

func (s *HTTPServer) listenAndServe() error {
	err := s.Server.ListenAndServe()

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
