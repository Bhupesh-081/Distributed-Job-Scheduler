package httpapi

import (
	"errors"
	"net/http"

	"github.com/google/uuid"

	"distributed-job-scheduler/internal/store"
)

type dlqEntryResponse struct {
	ID           string  `json:"id"`
	JobID        string  `json:"job_id"`
	QueueID      *string `json:"queue_id,omitempty"`
	FinalError   *string `json:"final_error,omitempty"`
	RetriesCount int     `json:"retries_count"`
	MovedAt      string  `json:"moved_at"`
}

func toDLQEntryResponse(e store.DeadLetterEntry) dlqEntryResponse {
	out := dlqEntryResponse{
		ID: e.ID.String(), JobID: e.JobID.String(), FinalError: e.FinalError,
		RetriesCount: e.RetriesCount, MovedAt: e.MovedAt.Format(timeFormat),
	}
	if e.QueueID != nil {
		s := e.QueueID.String()
		out.QueueID = &s
	}
	return out
}

func (s *Server) handleListQueueDLQ(w http.ResponseWriter, r *http.Request) {
	queueID, ok := pathUUID(w, r, "queueId")
	if !ok {
		return
	}
	if _, ok := s.requireQueueMember(w, r, queueID); !ok {
		return
	}
	limit, offset := pageParams(r)
	entries, err := s.store.ListDLQForQueue(r.Context(), queueID, limit, offset)
	if err != nil {
		internalError(w, err)
		return
	}
	out := make([]dlqEntryResponse, len(entries))
	for i, e := range entries {
		out[i] = toDLQEntryResponse(e)
	}
	writeJSON(w, http.StatusOK, out)
}

// requireDLQMember loads a DLQ entry and confirms the caller is a member of
// its queue's organization. An entry with no queue_id (an unscoped legacy
// job) has no owning org to check against, so it's treated as not found
// rather than exposed to every authenticated user.
func (s *Server) requireDLQMember(w http.ResponseWriter, r *http.Request, id uuid.UUID) (store.DeadLetterEntry, bool) {
	entry, err := s.store.GetDLQEntry(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			notFound(w, "dead letter entry not found")
			return store.DeadLetterEntry{}, false
		}
		internalError(w, err)
		return store.DeadLetterEntry{}, false
	}
	if entry.QueueID == nil {
		notFound(w, "dead letter entry not found")
		return store.DeadLetterEntry{}, false
	}
	if _, ok := s.requireQueueMember(w, r, *entry.QueueID); !ok {
		return store.DeadLetterEntry{}, false
	}
	return entry, true
}

func (s *Server) handleGetDLQEntry(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "dlqId")
	if !ok {
		return
	}
	entry, ok := s.requireDLQMember(w, r, id)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, toDLQEntryResponse(entry))
}

func (s *Server) handleReplayDLQEntry(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "dlqId")
	if !ok {
		return
	}
	if _, ok := s.requireDLQMember(w, r, id); !ok {
		return
	}
	jobID, err := s.store.ReplayDLQEntry(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			notFound(w, "dead letter entry not found")
			return
		}
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"job_id": jobID.String(), "status": "queued"})
}

func (s *Server) handleDeleteDLQEntry(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "dlqId")
	if !ok {
		return
	}
	if _, ok := s.requireDLQMember(w, r, id); !ok {
		return
	}
	if err := s.store.DeleteDLQEntry(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			notFound(w, "dead letter entry not found")
			return
		}
		internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
