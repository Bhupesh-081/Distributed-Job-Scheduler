package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"distributed-job-scheduler/internal/store"
)

type retryPolicyResponse struct {
	ID               string `json:"id"`
	ProjectID        string `json:"project_id"`
	Name             string `json:"name"`
	Strategy         string `json:"strategy"`
	BaseDelaySeconds int    `json:"base_delay_seconds"`
	MaxDelaySeconds  *int   `json:"max_delay_seconds,omitempty"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
}

func toRetryPolicyResponse(p store.RetryPolicy) retryPolicyResponse {
	return retryPolicyResponse{
		ID:               p.ID.String(),
		ProjectID:        p.ProjectID.String(),
		Name:             p.Name,
		Strategy:         p.Strategy,
		BaseDelaySeconds: p.BaseDelaySeconds,
		MaxDelaySeconds:  p.MaxDelaySeconds,
		CreatedAt:        p.CreatedAt.Format(timeFormat),
		UpdatedAt:        p.UpdatedAt.Format(timeFormat),
	}
}

func validStrategy(s string) bool {
	return s == "fixed" || s == "linear" || s == "exponential"
}

// requireRetryPolicyMember loads the policy and confirms the caller is a
// member of its owning project's organization.
func (s *Server) requireRetryPolicyMember(w http.ResponseWriter, r *http.Request, id uuid.UUID) (store.RetryPolicy, bool) {
	userID, _ := userIDFromContext(r.Context())

	policy, err := s.store.GetRetryPolicy(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			notFound(w, "retry policy not found")
			return store.RetryPolicy{}, false
		}
		internalError(w, err)
		return store.RetryPolicy{}, false
	}
	project, err := s.store.GetProject(r.Context(), policy.ProjectID)
	if err != nil {
		internalError(w, err)
		return store.RetryPolicy{}, false
	}
	if _, err := s.store.GetMemberRole(r.Context(), project.OrgID, userID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			forbidden(w, "not a member of this retry policy's organization")
			return store.RetryPolicy{}, false
		}
		internalError(w, err)
		return store.RetryPolicy{}, false
	}
	return policy, true
}

type retryPolicyRequest struct {
	Name             string `json:"name"`
	Strategy         string `json:"strategy"`
	BaseDelaySeconds int    `json:"base_delay_seconds"`
	MaxDelaySeconds  *int   `json:"max_delay_seconds,omitempty"`
}

func (req retryPolicyRequest) validate() error {
	if strings.TrimSpace(req.Name) == "" {
		return errors.New("name is required")
	}
	if !validStrategy(req.Strategy) {
		return errors.New("strategy must be one of: fixed, linear, exponential")
	}
	if req.BaseDelaySeconds <= 0 {
		return errors.New("base_delay_seconds must be > 0")
	}
	if req.MaxDelaySeconds != nil && *req.MaxDelaySeconds < req.BaseDelaySeconds {
		return errors.New("max_delay_seconds must be >= base_delay_seconds")
	}
	return nil
}

func (s *Server) handleCreateRetryPolicy(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, "projectId")
	if !ok {
		return
	}
	project, ok := s.requireProjectMember(w, r, projectID)
	if !ok {
		return
	}

	var req retryPolicyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}
	if err := req.validate(); err != nil {
		badRequest(w, err.Error())
		return
	}

	policy, err := s.store.CreateRetryPolicy(r.Context(), store.NewRetryPolicy{
		ProjectID: project.ID, Name: strings.TrimSpace(req.Name), Strategy: req.Strategy,
		BaseDelaySeconds: req.BaseDelaySeconds, MaxDelaySeconds: req.MaxDelaySeconds,
	})
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			conflict(w, "a retry policy with this name already exists in the project")
			return
		}
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toRetryPolicyResponse(policy))
}

func (s *Server) handleListRetryPolicies(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, "projectId")
	if !ok {
		return
	}
	if _, ok := s.requireProjectMember(w, r, projectID); !ok {
		return
	}

	limit, offset := pageParams(r)
	policies, err := s.store.ListRetryPoliciesForProject(r.Context(), projectID, limit, offset)
	if err != nil {
		internalError(w, err)
		return
	}
	out := make([]retryPolicyResponse, len(policies))
	for i, p := range policies {
		out[i] = toRetryPolicyResponse(p)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGetRetryPolicy(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "retryPolicyId")
	if !ok {
		return
	}
	policy, ok := s.requireRetryPolicyMember(w, r, id)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, toRetryPolicyResponse(policy))
}

func (s *Server) handleUpdateRetryPolicy(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "retryPolicyId")
	if !ok {
		return
	}
	if _, ok := s.requireRetryPolicyMember(w, r, id); !ok {
		return
	}

	var req retryPolicyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}
	if err := req.validate(); err != nil {
		badRequest(w, err.Error())
		return
	}

	policy, err := s.store.UpdateRetryPolicy(r.Context(), id, strings.TrimSpace(req.Name), req.Strategy, req.BaseDelaySeconds, req.MaxDelaySeconds)
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			conflict(w, "a retry policy with this name already exists in the project")
			return
		}
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toRetryPolicyResponse(policy))
}

func (s *Server) handleDeleteRetryPolicy(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "retryPolicyId")
	if !ok {
		return
	}
	if _, ok := s.requireRetryPolicyMember(w, r, id); !ok {
		return
	}
	if err := s.store.DeleteRetryPolicy(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			notFound(w, "retry policy not found")
			return
		}
		internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
