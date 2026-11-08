// Package web is the one HTTP server of the daemon. It serves nothing of
// its own: the app mounts /healthz, /metrics (sink/metrics) and /api/v1
// (sink/api). It is a poller.Sink only for its lifecycle (listen on
// Start, shut down on Close); put it last so the handlers it serves are
// started first.
package web

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/eduardpaul/routerctl/routerctl/internal/service/events"
	"github.com/eduardpaul/routerctl/routerctl/internal/service/poller"
)

// Server is the HTTP server.
type Server struct {
	listen string
	mux    *http.ServeMux
	srv    *http.Server
	ln     net.Listener
}

// New builds a server for listen (host:port).
func New(listen string) *Server { return &Server{listen: listen, mux: http.NewServeMux()} }

// Handle mounts h on pattern (a net/http ServeMux pattern).
func (s *Server) Handle(pattern string, h http.Handler) { s.mux.Handle(pattern, h) }

// Handler is the mux (tests).
func (s *Server) Handler() http.Handler { return s.mux }

// Addr is the bound address once started.
func (s *Server) Addr() string {
	if s.ln == nil {
		return ""
	}
	return s.ln.Addr().String()
}

// Name implements poller.Sink.
func (s *Server) Name() string { return "http" }

// Start implements poller.Sink.
func (s *Server) Start(context.Context, poller.Control) error {
	ln, err := net.Listen("tcp", s.listen)
	if err != nil {
		return fmt.Errorf("http: %w", err)
	}
	s.ln = ln
	// No WriteTimeout: the event stream is long-lived (it has heartbeats)
	s.srv = &http.Server{Handler: s.mux, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, IdleTimeout: 2 * time.Minute}
	go func() {
		if err := s.srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("http server stopped", "err", err)
		}
	}()
	slog.Info("http listening", "addr", ln.Addr().String())
	return nil
}

// State implements poller.Sink.
func (s *Server) State(poller.DeviceState) {}

// Event implements poller.Sink.
func (s *Server) Event(events.Event) {}

// Close implements poller.Sink.
func (s *Server) Close(ctx context.Context) error {
	if s.srv == nil {
		return nil
	}
	return s.srv.Shutdown(ctx)
}

// RequireToken wraps h with a bearer-token check.
func RequireToken(token string, h http.Handler) http.Handler {
	want := []byte("Bearer " + token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), want) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="routerctl"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		h.ServeHTTP(w, r)
	})
}
