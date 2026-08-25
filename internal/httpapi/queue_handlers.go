package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"distributed-job-scheduler/internal/store"
)

type queueResponse struct {
	ID                   string  `json:"id"`
	ProjectID            string  `json:"project_id"`
	Name                 string  `json:"name"`
	Priority             int     `json:"priority"`
	ConcurrencyLimit     int     `json:"concurrency_limit"`
	Paused               bool    `json:"paused"`
	CreatedAt            string  `json:"created_at"`
	UpdatedAt            string  `json:"updated_at"`
	DefaultRetryPolicyID *string `json:"default_retry_policy_id,omitempty"`
}

func toQueueResponse(q store.Queue) queueResponse {
	out := queueResponse{
		ID:               q.ID.String(),
		ProjectID:        q.ProjectID.String(),
		Name:             q.Name,
		Priority:         q.Priority,
		ConcurrencyLimit: q.ConcurrencyLimit,
		Paused:           q.Paused,
		CreatedAt:        q.CreatedAt.Format(timeFormat),
		UpdatedAt:        q.UpdatedAt.Format(timeFormat),
	}
	if q.DefaultRetryPolicyID != nil {
		s := q.DefaultRetryPolicyID.String()
		out.DefaultRetryPolicyID = &s
	}
	return out
}

// requireQueueMember loads the queue and confirms the caller is a member of
// its owning project's organization.
func (s *Server) requireQueueMember(w http.ResponseWriter, r *http.Request, queueID uuid.UUID) (store.Queue, bool) {
	return requireQueueAccess(w, r, s.store, queueID)
}

// requireQueueAccess is the shared implementation behind requireQueueMember
// on both Server (cmd/api) and JobServer (job-service) — job-service has no
// Server of its own but needs the same org-membership check on the queue a
// job belongs to.
func requireQueueAccess(w http.ResponseWriter, r *http.Request, st *store.Store, queueID uuid.UUID) (store.Queue, bool) {
	userID, _ := userIDFromContext(r.Context())

	queue, err := st.GetQueue(r.Context(), queueID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			notFound(w, "queue not found")
			return store.Queue{}, false
		}
		internalError(w, err)
		return store.Queue{}, false
	}
	project, err := st.GetProject(r.Context(), queue.ProjectID)
	if err != nil {
		internalError(w, err)
		return store.Queue{}, false
	}
	if _, err := st.GetMemberRole(r.Context(), project.OrgID, userID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			forbidden(w, "not a member of this queue's organization")
			return store.Queue{}, false
		}
		internalError(w, err)
		return store.Queue{}, false
	}
	return queue, true
}

type createQueueRequest struct {
	Name             string `json:"name"`
	Priority         int    `json:"priority"`
	ConcurrencyLimit int    `json:"concurrency_limit"`
}

func (s *Server) handleCreateQueue(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, "projectId")
	if !ok {
		return
	}
	project, ok := s.requireProjectMember(w, r, projectID)
	if !ok {
		return
	}

	var req createQueueRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		badRequest(w, "name is required")
		return
	}
	if req.ConcurrencyLimit <= 0 {
		req.ConcurrencyLimit = 5
	}

	queue, err := s.store.CreateQueue(r.Context(), store.NewQueue{
		ProjectID:        project.ID,
		Name:             req.Name,
		Priority:         req.Priority,
		ConcurrencyLimit: req.ConcurrencyLimit,
	})
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			conflict(w, "a queue with this name already exists in the project")
			return
		}
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toQueueResponse(queue))
}

func (s *Server) handleListQueues(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, "projectId")
	if !ok {
		return
	}
	if _, ok := s.requireProjectMember(w, r, projectID); !ok {
		return
	}

	limit, offset := pageParams(r)
	queues, err := s.store.ListQueuesForProject(r.Context(), projectID, limit, offset)
	if err != nil {
		internalError(w, err)
		return
	}
	out := make([]queueResponse, len(queues))
	for i, q := range queues {
		out[i] = toQueueResponse(q)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGetQueue(w http.ResponseWriter, r *http.Request) {
	queueID, ok := pathUUID(w, r, "queueId")
	if !ok {
		return
	}
	queue, ok := s.requireQueueMember(w, r, queueID)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, toQueueResponse(queue))
}

type updateQueueRequest struct {
	Name                 string `json:"name"`
	Priority             int    `json:"priority"`
	ConcurrencyLimit     int    `json:"concurrency_limit"`
	DefaultRetryPolicyID string `json:"default_retry_policy_id,omitempty"`
}

func (s *Server) handleUpdateQueue(w http.ResponseWriter, r *http.Request) {
	queueID, ok := pathUUID(w, r, "queueId")
	if !ok {
		return
	}
	if _, ok := s.requireQueueMember(w, r, queueID); !ok {
		return
	}

	var req updateQueueRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		badRequest(w, "name is required")
		return
	}
	if req.ConcurrencyLimit <= 0 {
		badRequest(w, "concurrency_limit must be > 0")
		return
	}
	var retryPolicyID *uuid.UUID
	if req.DefaultRetryPolicyID != "" {
		id, err := uuid.Parse(req.DefaultRetryPolicyID)
		if err != nil {
			badRequest(w, "default_retry_policy_id must be a valid UUID")
			return
		}
		retryPolicyID = &id
	}

	queue, err := s.store.UpdateQueueConfig(r.Context(), queueID, req.Name, req.Priority, req.ConcurrencyLimit, retryPolicyID)
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			conflict(w, "a queue with this name already exists in the project")
			return
		}
		if errors.Is(err, store.ErrRetryPolicyNotFound) {
			badRequest(w, "retry policy not found")
			return
		}
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toQueueResponse(queue))
}

func (s *Server) handleDeleteQueue(w http.ResponseWriter, r *http.Request) {
	queueID, ok := pathUUID(w, r, "queueId")
	if !ok {
		return
	}
	if _, ok := s.requireQueueMember(w, r, queueID); !ok {
		return
	}
	if err := s.store.DeleteQueue(r.Context(), queueID); err != nil {
		if errors.Is(err, store.ErrConflict) {
			conflict(w, "queue still has jobs; delete or reassign them first")
			return
		}
		if errors.Is(err, store.ErrNotFound) {
			notFound(w, "queue not found")
			return
		}
		internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handlePauseQueue(w http.ResponseWriter, r *http.Request) {
	s.setQueuePaused(w, r, true)
}

func (s *Server) handleResumeQueue(w http.ResponseWriter, r *http.Request) {
	s.setQueuePaused(w, r, false)
}

func (s *Server) setQueuePaused(w http.ResponseWriter, r *http.Request, paused bool) {
	queueID, ok := pathUUID(w, r, "queueId")
	if !ok {
		return
	}
	if _, ok := s.requireQueueMember(w, r, queueID); !ok {
		return
	}
	queue, err := s.store.SetQueuePaused(r.Context(), queueID, paused)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toQueueResponse(queue))
}

type queueStatsResponse struct {
	Queued    int `json:"queued"`
	Running   int `json:"running"`
	Success   int `json:"success"`
	Failed    int `json:"failed"`
	Dead      int `json:"dead"`
	Cancelled int `json:"cancelled"`
}

func (s *Server) handleQueueStats(w http.ResponseWriter, r *http.Request) {
	queueID, ok := pathUUID(w, r, "queueId")
	if !ok {
		return
	}
	if _, ok := s.requireQueueMember(w, r, queueID); !ok {
		return
	}
	stats, err := s.store.GetQueueStats(r.Context(), queueID)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, queueStatsResponse{
		Queued: stats.Queued, Running: stats.Running, Success: stats.Success,
		Failed: stats.Failed, Dead: stats.Dead, Cancelled: stats.Cancelled,
	})
}
