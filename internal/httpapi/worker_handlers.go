package httpapi

import (
	"errors"
	"net/http"

	"distributed-job-scheduler/internal/store"
)

// Workers aren't project-scoped — the consumer-service pool is shared
// infrastructure, not owned by one org — so these endpoints are visible to
// any authenticated user rather than gated by org membership.

type workerResponse struct {
	ID              string  `json:"id"`
	Hostname        string  `json:"hostname"`
	PID             int     `json:"pid"`
	Concurrency     int     `json:"concurrency"`
	Status          string  `json:"status"`
	StartedAt       string  `json:"started_at"`
	StoppedAt       *string `json:"stopped_at,omitempty"`
	LastHeartbeatAt string  `json:"last_heartbeat_at"`
}

func toWorkerResponse(w store.Worker) workerResponse {
	out := workerResponse{
		ID: w.ID.String(), Hostname: w.Hostname, PID: w.PID, Concurrency: w.Concurrency,
		Status: w.Status, StartedAt: w.StartedAt.Format(timeFormat), LastHeartbeatAt: w.LastHeartbeatAt.Format(timeFormat),
	}
	if w.StoppedAt != nil {
		s := w.StoppedAt.Format(timeFormat)
		out.StoppedAt = &s
	}
	return out
}

func (s *Server) handleListWorkers(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	limit, offset := pageParams(r)
	workers, err := s.store.ListWorkers(r.Context(), status, limit, offset)
	if err != nil {
		internalError(w, err)
		return
	}
	out := make([]workerResponse, len(workers))
	for i, wk := range workers {
		out[i] = toWorkerResponse(wk)
	}
	writeJSON(w, http.StatusOK, out)
}

type workerHeartbeatResponse struct {
	HeartbeatAt   string `json:"heartbeat_at"`
	InFlightCount int    `json:"in_flight_count"`
}

type workerDetailResponse struct {
	workerResponse
	RecentHeartbeats []workerHeartbeatResponse `json:"recent_heartbeats"`
}

const recentHeartbeatLimit = 20

func (s *Server) handleGetWorker(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "workerId")
	if !ok {
		return
	}
	worker, err := s.store.GetWorker(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			notFound(w, "worker not found")
			return
		}
		internalError(w, err)
		return
	}
	heartbeats, err := s.store.ListWorkerHeartbeats(r.Context(), id, recentHeartbeatLimit)
	if err != nil {
		internalError(w, err)
		return
	}
	out := workerDetailResponse{workerResponse: toWorkerResponse(worker)}
	out.RecentHeartbeats = make([]workerHeartbeatResponse, len(heartbeats))
	for i, h := range heartbeats {
		out.RecentHeartbeats[i] = workerHeartbeatResponse{HeartbeatAt: h.HeartbeatAt.Format(timeFormat), InFlightCount: h.InFlightCount}
	}
	writeJSON(w, http.StatusOK, out)
}
