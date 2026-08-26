// Package httpapi wires the REST API: routing, auth middleware, and handlers
// for auth, organizations, and projects.
package httpapi

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"

	"distributed-job-scheduler/internal/authsvc"
	"distributed-job-scheduler/internal/heartbeat"
	"distributed-job-scheduler/internal/mailer"
	"distributed-job-scheduler/internal/store"
)

type Server struct {
	store           *store.Store
	tokens          authsvc.TokenIssuer
	refreshTokenTTL time.Duration
	redis           *redis.Client
	mailer          *mailer.Mailer
	mux             *http.ServeMux
}

func NewServer(st *store.Store, tokens authsvc.TokenIssuer, refreshTokenTTL time.Duration, rdb *redis.Client, mail *mailer.Mailer) *Server {
	s := &Server{store: st, tokens: tokens, refreshTokenTTL: refreshTokenTTL, redis: rdb, mailer: mail, mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	logRequests(corsMiddleware(s.mux)).ServeHTTP(w, r)
}

func (s *Server) routes() {
	s.mux.HandleFunc("POST /auth/register", s.handleRegister)
	s.mux.HandleFunc("POST /auth/login", s.handleLogin)
	s.mux.HandleFunc("POST /auth/refresh", s.handleRefresh)
	s.mux.HandleFunc("POST /auth/logout", s.handleLogout)
	s.mux.HandleFunc("POST /auth/verify-email", s.handleVerifyEmail)
	s.mux.HandleFunc("POST /auth/resend-verification", s.handleResendVerification)
	s.mux.HandleFunc("POST /auth/forgot-password", s.handleForgotPassword)
	s.mux.HandleFunc("POST /auth/reset-password", s.handleResetPassword)
	s.mux.HandleFunc("GET /auth/me", s.requireAuth(s.handleGetMe))
	s.mux.HandleFunc("PATCH /auth/me", s.requireAuth(s.handleUpdateMe))
	s.mux.HandleFunc("POST /auth/change-password", s.requireAuth(s.handleChangePassword))

	s.mux.HandleFunc("GET /organizations", s.requireAuth(s.handleListOrganizations))
	s.mux.HandleFunc("POST /organizations", s.requireAuth(s.handleCreateOrganization))
	s.mux.HandleFunc("GET /organizations/{orgId}", s.requireAuth(s.handleGetOrganization))
	s.mux.HandleFunc("PATCH /organizations/{orgId}", s.requireAuth(s.handleUpdateOrganization))
	s.mux.HandleFunc("DELETE /organizations/{orgId}", s.requireAuth(s.handleDeleteOrganization))
	s.mux.HandleFunc("POST /organizations/{orgId}/members", s.requireAuth(s.handleAddOrgMember))

	s.mux.HandleFunc("GET /organizations/{orgId}/projects", s.requireAuth(s.handleListProjects))
	s.mux.HandleFunc("POST /organizations/{orgId}/projects", s.requireAuth(s.handleCreateProject))
	s.mux.HandleFunc("GET /projects/{projectId}", s.requireAuth(s.handleGetProject))
	s.mux.HandleFunc("PATCH /projects/{projectId}", s.requireAuth(s.handleUpdateProject))
	s.mux.HandleFunc("DELETE /projects/{projectId}", s.requireAuth(s.handleDeleteProject))

	s.mux.HandleFunc("GET /projects/{projectId}/queues", s.requireAuth(s.handleListQueues))
	s.mux.HandleFunc("POST /projects/{projectId}/queues", s.requireAuth(s.handleCreateQueue))
	s.mux.HandleFunc("GET /queues/{queueId}", s.requireAuth(s.handleGetQueue))
	s.mux.HandleFunc("PATCH /queues/{queueId}", s.requireAuth(s.handleUpdateQueue))
	s.mux.HandleFunc("DELETE /queues/{queueId}", s.requireAuth(s.handleDeleteQueue))
	s.mux.HandleFunc("POST /queues/{queueId}/pause", s.requireAuth(s.handlePauseQueue))
	s.mux.HandleFunc("POST /queues/{queueId}/resume", s.requireAuth(s.handleResumeQueue))
	s.mux.HandleFunc("GET /queues/{queueId}/stats", s.requireAuth(s.handleQueueStats))

	s.mux.HandleFunc("GET /queues/{queueId}/scheduled-jobs", s.requireAuth(s.handleListScheduledJobs))
	s.mux.HandleFunc("POST /queues/{queueId}/scheduled-jobs", s.requireAuth(s.handleCreateScheduledJob))
	s.mux.HandleFunc("GET /scheduled-jobs/{scheduledJobId}", s.requireAuth(s.handleGetScheduledJob))
	s.mux.HandleFunc("PATCH /scheduled-jobs/{scheduledJobId}", s.requireAuth(s.handleUpdateScheduledJob))
	s.mux.HandleFunc("DELETE /scheduled-jobs/{scheduledJobId}", s.requireAuth(s.handleDeleteScheduledJob))
	s.mux.HandleFunc("POST /scheduled-jobs/{scheduledJobId}/pause", s.requireAuth(s.handlePauseScheduledJob))
	s.mux.HandleFunc("POST /scheduled-jobs/{scheduledJobId}/resume", s.requireAuth(s.handleResumeScheduledJob))

	s.mux.HandleFunc("GET /projects/{projectId}/retry-policies", s.requireAuth(s.handleListRetryPolicies))
	s.mux.HandleFunc("POST /projects/{projectId}/retry-policies", s.requireAuth(s.handleCreateRetryPolicy))
	s.mux.HandleFunc("GET /retry-policies/{retryPolicyId}", s.requireAuth(s.handleGetRetryPolicy))
	s.mux.HandleFunc("PATCH /retry-policies/{retryPolicyId}", s.requireAuth(s.handleUpdateRetryPolicy))
	s.mux.HandleFunc("DELETE /retry-policies/{retryPolicyId}", s.requireAuth(s.handleDeleteRetryPolicy))

	s.mux.HandleFunc("GET /workers", s.requireAuth(s.handleListWorkers))
	s.mux.HandleFunc("GET /workers/{workerId}", s.requireAuth(s.handleGetWorker))

	s.mux.HandleFunc("GET /queues/{queueId}/dlq", s.requireAuth(s.handleListQueueDLQ))
	s.mux.HandleFunc("GET /dlq/{dlqId}", s.requireAuth(s.handleGetDLQEntry))
	s.mux.HandleFunc("POST /dlq/{dlqId}/replay", s.requireAuth(s.handleReplayDLQEntry))
	s.mux.HandleFunc("DELETE /dlq/{dlqId}", s.requireAuth(s.handleDeleteDLQEntry))

	s.mux.HandleFunc("GET /system/health", s.handleHealth)
	s.mux.HandleFunc("GET /system/metrics", s.requireAuth(s.handleSystemMetrics))
}

type healthResponse struct {
	Status         string                `json:"status"`
	WatcherService watcherHealthResponse `json:"watcher_service"`
}

type watcherHealthResponse struct {
	Alive                bool    `json:"alive"`
	LastPollAt           *string `json:"last_poll_at,omitempty"`
	SecondsSinceLastPoll *int    `json:"seconds_since_last_poll,omitempty"`
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	resp := healthResponse{Status: "ok"}

	status, err := heartbeat.Get(r.Context(), s.redis)
	if err != nil {
		slog.Error("get watcher heartbeat", "error", err)
	} else {
		resp.WatcherService.Alive = status.Alive
		if !status.LastPollAt.IsZero() {
			t := status.LastPollAt.Format(timeFormat)
			resp.WatcherService.LastPollAt = &t
			secs := int(time.Since(status.LastPollAt).Seconds())
			resp.WatcherService.SecondsSinceLastPoll = &secs
		}
	}

	writeJSON(w, http.StatusOK, resp)
}
