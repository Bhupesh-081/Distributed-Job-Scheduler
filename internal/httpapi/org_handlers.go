package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/mail"
	"strings"

	"github.com/google/uuid"

	"distributed-job-scheduler/internal/store"
)

type organizationResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	OwnerUserID string `json:"owner_user_id"`
	CreatedAt   string `json:"created_at"`
}

func toOrgResponse(o store.Organization) organizationResponse {
	return organizationResponse{
		ID:          o.ID.String(),
		Name:        o.Name,
		OwnerUserID: o.OwnerUserID.String(),
		CreatedAt:   o.CreatedAt.Format(timeFormat),
	}
}

const timeFormat = "2006-01-02T15:04:05Z07:00"

// pathUUID parses a path parameter as a UUID, writing a 400 and returning ok=false on failure.
func pathUUID(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		badRequest(w, "invalid "+name)
		return uuid.Nil, false
	}
	return id, true
}

type createOrgRequest struct {
	Name string `json:"name"`
}

func (s *Server) handleCreateOrganization(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFromContext(r.Context())

	var req createOrgRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		badRequest(w, "name is required")
		return
	}

	org, err := s.store.CreateOrganization(r.Context(), req.Name, userID)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toOrgResponse(org))
}

func (s *Server) handleListOrganizations(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFromContext(r.Context())
	limit, offset := pageParams(r)

	orgs, err := s.store.ListOrganizationsForUser(r.Context(), userID, limit, offset)
	if err != nil {
		internalError(w, err)
		return
	}
	out := make([]organizationResponse, len(orgs))
	for i, o := range orgs {
		out[i] = toOrgResponse(o)
	}
	writeJSON(w, http.StatusOK, out)
}

// requireOrgMember loads the org and confirms the caller is a member,
// returning ok=false (response already written) otherwise.
func (s *Server) requireOrgMember(w http.ResponseWriter, r *http.Request, orgID, userID uuid.UUID) (store.Organization, bool) {
	org, err := s.store.GetOrganization(r.Context(), orgID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			notFound(w, "organization not found")
			return store.Organization{}, false
		}
		internalError(w, err)
		return store.Organization{}, false
	}
	if _, err := s.store.GetMemberRole(r.Context(), orgID, userID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			forbidden(w, "not a member of this organization")
			return store.Organization{}, false
		}
		internalError(w, err)
		return store.Organization{}, false
	}
	return org, true
}

func (s *Server) handleGetOrganization(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFromContext(r.Context())
	orgID, ok := pathUUID(w, r, "orgId")
	if !ok {
		return
	}
	org, ok := s.requireOrgMember(w, r, orgID, userID)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, toOrgResponse(org))
}

type updateOrgRequest struct {
	Name string `json:"name"`
}

func (s *Server) handleUpdateOrganization(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFromContext(r.Context())
	orgID, ok := pathUUID(w, r, "orgId")
	if !ok {
		return
	}
	if !s.requireOrgOwner(w, r, orgID, userID) {
		return
	}

	var req updateOrgRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		badRequest(w, "name is required")
		return
	}

	org, err := s.store.UpdateOrganizationName(r.Context(), orgID, req.Name)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			notFound(w, "organization not found")
			return
		}
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toOrgResponse(org))
}

func (s *Server) handleDeleteOrganization(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFromContext(r.Context())
	orgID, ok := pathUUID(w, r, "orgId")
	if !ok {
		return
	}
	if !s.requireOrgOwner(w, r, orgID, userID) {
		return
	}

	if err := s.store.DeleteOrganization(r.Context(), orgID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			notFound(w, "organization not found")
			return
		}
		internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// requireOrgOwner writes an error response and returns false unless the
// caller is the org's owner.
func (s *Server) requireOrgOwner(w http.ResponseWriter, r *http.Request, orgID, userID uuid.UUID) bool {
	role, err := s.store.GetMemberRole(r.Context(), orgID, userID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			forbidden(w, "not a member of this organization")
			return false
		}
		internalError(w, err)
		return false
	}
	if role != "owner" {
		forbidden(w, "only the organization owner can do this")
		return false
	}
	return true
}

type addMemberRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

func (s *Server) handleAddOrgMember(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFromContext(r.Context())
	orgID, ok := pathUUID(w, r, "orgId")
	if !ok {
		return
	}
	if !s.requireOrgOwner(w, r, orgID, userID) {
		return
	}

	var req addMemberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}
	if req.Role == "" {
		req.Role = "member"
	}
	if req.Role != "owner" && req.Role != "member" {
		badRequest(w, "role must be 'owner' or 'member'")
		return
	}
	addr, err := mail.ParseAddress(req.Email)
	if err != nil {
		badRequest(w, "invalid email")
		return
	}

	member, err := s.store.GetUserByEmail(r.Context(), addr.Address)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			notFound(w, "no user with that email")
			return
		}
		internalError(w, err)
		return
	}

	if err := s.store.AddOrgMember(r.Context(), orgID, member.ID, req.Role); err != nil {
		if errors.Is(err, store.ErrConflict) {
			conflict(w, "user is already a member")
			return
		}
		internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
