package httpapi

import (
	"net/http"

	"github.com/redis/go-redis/v9"

	"distributed-job-scheduler/internal/store"
)

// JobServer is the MVP job-service API: unauthenticated for now (see
// "Auth on job-service routes" in docs/design-decisions.md's MVP bootstrap
// ledger) and not project/queue-scoped yet.
type JobServer struct {
	store *store.Store
	redis *redis.Client
	mux   *http.ServeMux
}

func NewJobServer(st *store.Store, rdb *redis.Client) *JobServer {
	s := &JobServer{store: st, redis: rdb, mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *JobServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	logRequests(corsMiddleware(s.mux)).ServeHTTP(w, r)
}

func (s *JobServer) routes() {
	s.mux.HandleFunc("POST /jobs", s.handleCreateJob)
	s.mux.HandleFunc("POST /jobs/batch", s.handleCreateJobsBatch)
	s.mux.HandleFunc("GET /jobs", s.handleListJobs)
	s.mux.HandleFunc("GET /jobs/{id}", s.handleGetJob)
	s.mux.HandleFunc("POST /jobs/{id}/cancel", s.handleCancelJob)
	s.mux.HandleFunc("GET /system/health", s.handleHealth)
}

func (s *JobServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
