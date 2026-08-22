// Package httpapi wires the REST API: routing, auth middleware, and handlers
// for auth, organizations, and projects.
package httpapi

import (
	"net/http"
	"time"

	"distributed-job-scheduler/internal/authsvc"
	"distributed-job-scheduler/internal/store"
)

type Server struct {
	store           *store.Store
	tokens          authsvc.TokenIssuer
	refreshTokenTTL time.Duration
	mux             *http.ServeMux
}

func NewServer(st *store.Store, tokens authsvc.TokenIssuer, refreshTokenTTL time.Duration) *Server {
	s := &Server{store: st, tokens: tokens, refreshTokenTTL: refreshTokenTTL, mux: http.NewServeMux()}
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

	s.mux.HandleFunc("GET /system/health", s.handleHealth)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
