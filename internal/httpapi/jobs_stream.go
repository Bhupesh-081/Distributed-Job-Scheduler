package httpapi

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// upgrader relaxes the origin check the same way corsMiddleware relaxes
// CORS for REST: auth is bearer/query-token-only, not cookie-based, so a
// cross-origin dashboard carries no CSRF risk here.
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// handleJobsStream pushes job changes for one queue over a WebSocket
// instead of making every dashboard tab poll GET /jobs on its own. It's
// server-side polling underneath (ListJobsModifiedSince every tick,
// broadcast to whoever's connected) rather than Postgres LISTEN/NOTIFY -
// simpler and correct given the existing modified_time column, at the cost
// of up to one tick (~1.5s) of latency. ponytail: NOTIFY would push
// instantly instead of polling; revisit if 1.5s ever isn't "live" enough.
//
// Browsers' native WebSocket API can't set an Authorization header, so the
// token travels as a query param here instead - the only route that needs
// this exception.
func (s *JobServer) handleJobsStream(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		token = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	}
	userID, err := s.tokens.ParseAccessToken(token)
	if err != nil {
		unauthorized(w, "invalid or expired token")
		return
	}
	r = r.WithContext(context.WithValue(r.Context(), userIDKey, userID))

	queueID, err := uuid.Parse(r.URL.Query().Get("queue_id"))
	if err != nil {
		badRequest(w, "queue_id is required and must be a valid UUID")
		return
	}
	if _, ok := requireQueueAccess(w, r, s.store, queueID); !ok {
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return // Upgrade already wrote the error response
	}
	defer conn.Close()

	// WebSocket requires reading control frames (pings/close) even when we
	// never expect a client message; this also detects client disconnects.
	closed := make(chan struct{})
	go func() {
		defer close(closed)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	ticker := time.NewTicker(1500 * time.Millisecond)
	defer ticker.Stop()
	since := time.Now()

	for {
		select {
		case <-closed:
			return
		case <-r.Context().Done():
			return
		case <-ticker.C:
			queriedAt := time.Now()
			jobs, err := s.store.ListJobsModifiedSince(r.Context(), queueID, since)
			if err != nil {
				return
			}
			since = queriedAt
			for _, j := range jobs {
				if err := conn.WriteJSON(map[string]any{"type": "job_updated", "job": toJobResponse(j)}); err != nil {
					return
				}
			}
		}
	}
}
