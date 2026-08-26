package httpapi

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"distributed-job-scheduler/internal/authsvc"
)

type ctxKey int

const userIDKey ctxKey = iota

func userIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(userIDKey).(uuid.UUID)
	return id, ok
}

// requireAuth validates the Bearer JWT and injects the caller's user ID into the request context.
func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return requireAuthWith(s.tokens, next)
}

// requireAuth is JobServer's counterpart to Server.requireAuth. job-service
// is a separate binary/mux but shares the same JWT secret/issuer, so a
// token from cmd/api's /auth/login works here too.
func (s *JobServer) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return requireAuthWith(s.tokens, next)
}

func requireAuthWith(tokens authsvc.TokenIssuer, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		token, ok := strings.CutPrefix(header, "Bearer ")
		if !ok || token == "" {
			unauthorized(w, "missing bearer token")
			return
		}
		userID, err := tokens.ParseAccessToken(token)
		if err != nil {
			unauthorized(w, "invalid or expired token")
			return
		}
		ctx := context.WithValue(r.Context(), userIDKey, userID)
		next(w, r.WithContext(ctx))
	}
}

// corsMiddleware allows browser-based clients (the dashboard) to call the
// API from a different origin. Auth is bearer-token-only, not cookie-based,
// so a wildcard origin carries no CSRF risk.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		slog.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// Hijack forwards to the underlying ResponseWriter so /jobs/stream's
// WebSocket upgrade (which needs to take over the raw connection) still
// works wrapped in this logging middleware.
func (r *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hj, ok := r.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("underlying ResponseWriter does not support hijacking")
	}
	return hj.Hijack()
}
