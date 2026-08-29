package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"distributed-job-scheduler/internal/store"
)

type scriptResponse struct {
	ID         string `json:"id"`
	ProjectID  string `json:"project_id"`
	Name       string `json:"name"`
	ScriptType string `json:"script_type"`
	Content    string `json:"content"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

func toScriptResponse(s store.Script) scriptResponse {
	return scriptResponse{
		ID:         s.ID.String(),
		ProjectID:  s.ProjectID.String(),
		Name:       s.Name,
		ScriptType: s.ScriptType,
		Content:    s.Content,
		CreatedAt:  s.CreatedAt.Format(timeFormat),
		UpdatedAt:  s.UpdatedAt.Format(timeFormat),
	}
}

func validScriptType(t string) bool {
	return t == "python" || t == "bash"
}

// requireScriptMember loads the script and confirms the caller is a member
// of its owning project's organization.
func (s *Server) requireScriptMember(w http.ResponseWriter, r *http.Request, id uuid.UUID) (store.Script, bool) {
	userID, _ := userIDFromContext(r.Context())

	script, err := s.store.GetScript(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			notFound(w, "script not found")
			return store.Script{}, false
		}
		internalError(w, err)
		return store.Script{}, false
	}
	project, err := s.store.GetProject(r.Context(), script.ProjectID)
	if err != nil {
		internalError(w, err)
		return store.Script{}, false
	}
	if _, err := s.store.GetMemberRole(r.Context(), project.OrgID, userID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			forbidden(w, "not a member of this script's organization")
			return store.Script{}, false
		}
		internalError(w, err)
		return store.Script{}, false
	}
	return script, true
}

type scriptRequest struct {
	Name       string `json:"name"`	
	ScriptType string `json:"script_type"`
	Content    string `json:"content"`
}

func (req scriptRequest) validate() error {
	if strings.TrimSpace(req.Name) == "" {
		return errors.New("name is required")
	}
	if !validScriptType(req.ScriptType) {
		return errors.New("script_type must be one of: python, bash")
	}
	if strings.TrimSpace(req.Content) == "" {
		return errors.New("content is required")
	}
	return nil
}

func (s *Server) handleCreateScript(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, "projectId")
	if !ok {
		return
	}
	project, ok := s.requireProjectMember(w, r, projectID)
	if !ok {
		return
	}

	var req scriptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}
	if err := req.validate(); err != nil {
		badRequest(w, err.Error())
		return
	}

	script, err := s.store.CreateScript(r.Context(), store.NewScript{
		ProjectID: project.ID, Name: strings.TrimSpace(req.Name), ScriptType: req.ScriptType, Content: req.Content,
	})
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			conflict(w, "a script with this name already exists in the project")
			return
		}
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toScriptResponse(script))
}

func (s *Server) handleListScripts(w http.ResponseWriter, r *http.Request) {
	projectID, ok := pathUUID(w, r, "projectId")
	if !ok {
		return
	}
	if _, ok := s.requireProjectMember(w, r, projectID); !ok {
		return
	}

	limit, offset := pageParams(r)
	scripts, err := s.store.ListScriptsForProject(r.Context(), projectID, limit, offset)
	if err != nil {
		internalError(w, err)
		return
	}
	out := make([]scriptResponse, len(scripts))
	for i, sc := range scripts {
		out[i] = toScriptResponse(sc)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleUpdateScript(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "scriptId")
	if !ok {
		return
	}
	if _, ok := s.requireScriptMember(w, r, id); !ok {
		return
	}

	var req scriptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}
	if err := req.validate(); err != nil {
		badRequest(w, err.Error())
		return
	}

	script, err := s.store.UpdateScript(r.Context(), id, strings.TrimSpace(req.Name), req.ScriptType, req.Content)
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			conflict(w, "a script with this name already exists in the project")
			return
		}
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toScriptResponse(script))
}

func (s *Server) handleDeleteScript(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "scriptId")
	if !ok {
		return
	}
	if _, ok := s.requireScriptMember(w, r, id); !ok {
		return
	}
	if err := s.store.DeleteScript(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			notFound(w, "script not found")
			return
		}
		internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
