package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"distributed-job-scheduler/internal/store"
)

type projectResponse struct {
	ID        string `json:"id"`
	OrgID     string `json:"org_id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

func toProjectResponse(p store.Project) projectResponse {
	return projectResponse{
		ID:        p.ID.String(),
		OrgID:     p.OrgID.String(),
		Name:      p.Name,
		CreatedAt: p.CreatedAt.Format(timeFormat),
	}
}

type createProjectRequest struct {
	Name string `json:"name"`
}

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFromContext(r.Context())
	orgID, ok := pathUUID(w, r, "orgId")
	if !ok {
		return
	}
	if _, ok := s.requireOrgMember(w, r, orgID, userID); !ok {
		return
	}

	var req createProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		badRequest(w, "name is required")
		return
	}

	project, err := s.store.CreateProject(r.Context(), orgID, req.Name)
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			conflict(w, "a project with this name already exists in the organization")
			return
		}
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toProjectResponse(project))
}

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	userID, _ := userIDFromContext(r.Context())
	orgID, ok := pathUUID(w, r, "orgId")
	if !ok {
		return
	}
	if _, ok := s.requireOrgMember(w, r, orgID, userID); !ok {
		return
	}

	limit, offset := pageParams(r)
	projects, err := s.store.ListProjectsForOrg(r.Context(), orgID, limit, offset)
	if err != nil {
		internalError(w, err)
		return
	}
	out := make([]projectResponse, len(projects))
	for i, p := range projects {
		out[i] = toProjectResponse(p)
	}
	writeJSON(w, http.StatusOK, out)
}

// requireProjectMember loads the project and confirms the caller is a member
// of its owning organization.
func (s *Server) requireProjectMember(w http.ResponseWriter, r *http.Request, projectID uuid.UUID) (store.Project, bool) {
	userID, _ := userIDFromContext(r.Context())

	project, err := s.store.GetProject(r.Context(), projectID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			notFound(w, "project not found")
			return store.Project{}, false
		}
		internalError(w, err)
		return store.Project{}, false
	}
	if _, err := s.store.GetMemberRole(r.Context(), project.OrgID, userID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			forbidden(w, "not a member of this project's organization")
			return store.Project{}, false
		}
		internalError(w, err)
		return store.Project{}, false
	}
	return project, true
}

func (s *Server) handleGetProject(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, "projectId")
	if !ok {
		return
	}
	project, ok := s.requireProjectMember(w, r, projectID)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, toProjectResponse(project))
}

type updateProjectRequest struct {
	Name string `json:"name"`
}

func (s *Server) handleUpdateProject(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, "projectId")
	if !ok {
		return
	}
	if _, ok := s.requireProjectMember(w, r, projectID); !ok {
		return
	}

	var req updateProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		badRequest(w, "name is required")
		return
	}

	project, err := s.store.UpdateProjectName(r.Context(), projectID, req.Name)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			notFound(w, "project not found")
			return
		}
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toProjectResponse(project))
}

func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, "projectId")
	if !ok {
		return
	}
	if _, ok := s.requireProjectMember(w, r, projectID); !ok {
		return
	}

	if err := s.store.DeleteProject(r.Context(), projectID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			notFound(w, "project not found")
			return
		}
		internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
