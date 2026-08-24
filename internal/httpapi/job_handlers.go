package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"distributed-job-scheduler/internal/cancel"
	"distributed-job-scheduler/internal/store"
)

type createJobRequest struct {
	Name          string          `json:"name"`
	ScheduledType string          `json:"scheduled_type"`
	DelaySeconds  *int            `json:"delay_seconds,omitempty"`  // scheduled_type=delayed
	ScheduledTime *string         `json:"scheduled_time,omitempty"` // scheduled_type=scheduled, RFC3339
	Payload       json.RawMessage `json:"payload"`
	RetriesMax    *int            `json:"retries_max,omitempty"`
}

// parseNewJob validates a createJobRequest and turns it into a store.NewJob.
// Pure (takes "now" as input) so it's testable without a database.
func parseNewJob(req createJobRequest, now time.Time) (store.NewJob, error) {
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return store.NewJob{}, fmt.Errorf("name is required")
	}
	if len(req.Payload) == 0 || string(req.Payload) == "null" {
		return store.NewJob{}, fmt.Errorf("payload is required")
	}
	if !json.Valid(req.Payload) {
		return store.NewJob{}, fmt.Errorf("payload must be valid JSON")
	}

	retriesMax := 3
	if req.RetriesMax != nil {
		if *req.RetriesMax < 0 {
			return store.NewJob{}, fmt.Errorf("retries_max must be >= 0")
		}
		retriesMax = *req.RetriesMax
	}

	out := store.NewJob{
		Name:          req.Name,
		ScheduledType: req.ScheduledType,
		Payload:       req.Payload,
		RetriesMax:    retriesMax,
	}

	switch req.ScheduledType {
	case "immediate":
		t := now
		out.ScheduledTime = &t
	case "delayed":
		if req.DelaySeconds == nil || *req.DelaySeconds <= 0 {
			return store.NewJob{}, fmt.Errorf("delay_seconds must be > 0 for a delayed job")
		}
		t := now.Add(time.Duration(*req.DelaySeconds) * time.Second)
		out.ScheduledTime = &t
	case "scheduled":
		if req.ScheduledTime == nil {
			return store.NewJob{}, fmt.Errorf("scheduled_time is required for a scheduled job")
		}
		t, err := time.Parse(time.RFC3339, *req.ScheduledTime)
		if err != nil {
			return store.NewJob{}, fmt.Errorf("scheduled_time must be RFC3339")
		}
		if !t.After(now) {
			return store.NewJob{}, fmt.Errorf("scheduled_time must be in the future")
		}
		out.ScheduledTime = &t
	case "recurring":
		return store.NewJob{}, fmt.Errorf("recurring jobs are not supported yet")
	default:
		return store.NewJob{}, fmt.Errorf("scheduled_type must be one of: immediate, delayed, scheduled")
	}

	return out, nil
}

type jobResponse struct {
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	ScheduledType  string          `json:"scheduled_type"`
	Status         string          `json:"status"`
	ScheduledTime  *string         `json:"scheduled_time,omitempty"`
	CronExpression *string         `json:"cron_expression,omitempty"`
	Payload        json.RawMessage `json:"payload"`
	RetriesCount   int             `json:"retries_count"`
	RetriesMax     int             `json:"retries_max"`
	CreatedAt      string          `json:"created_at"`
}

func toJobResponse(j store.Job) jobResponse {
	out := jobResponse{
		ID:            j.ID.String(),
		Name:          j.Name,
		ScheduledType: j.ScheduledType,
		Status:        j.Status,
		Payload:       j.Payload,
		RetriesCount:  j.RetriesCount,
		RetriesMax:    j.RetriesMax,
		CreatedAt:     j.CreatedAt.Format(timeFormat),
	}
	if j.ScheduledTime != nil {
		s := j.ScheduledTime.Format(timeFormat)
		out.ScheduledTime = &s
	}
	out.CronExpression = j.CronExpression
	return out
}

func (s *JobServer) handleCreateJob(w http.ResponseWriter, r *http.Request) {
	var req createJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}
	newJob, err := parseNewJob(req, time.Now())
	if err != nil {
		badRequest(w, err.Error())
		return
	}
	job, err := s.store.CreateJob(r.Context(), newJob)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toJobResponse(job))
}

type createJobsBatchRequest struct {
	Jobs []createJobRequest `json:"jobs"`
}

func (s *JobServer) handleCreateJobsBatch(w http.ResponseWriter, r *http.Request) {
	var req createJobsBatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}
	if len(req.Jobs) == 0 {
		badRequest(w, "jobs must be a non-empty array")
		return
	}

	now := time.Now()
	newJobs := make([]store.NewJob, len(req.Jobs))
	for i, jr := range req.Jobs {
		nj, err := parseNewJob(jr, now)
		if err != nil {
			badRequest(w, fmt.Sprintf("jobs[%d]: %s", i, err.Error()))
			return
		}
		newJobs[i] = nj
	}

	jobs, err := s.store.CreateJobsBatch(r.Context(), newJobs)
	if err != nil {
		internalError(w, err)
		return
	}
	out := make([]jobResponse, len(jobs))
	for i, j := range jobs {
		out[i] = toJobResponse(j)
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *JobServer) handleListJobs(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	limit, offset := pageParams(r)
	jobs, err := s.store.ListJobs(r.Context(), status, limit, offset)
	if err != nil {
		internalError(w, err)
		return
	}
	out := make([]jobResponse, len(jobs))
	for i, j := range jobs {
		out[i] = toJobResponse(j)
	}
	writeJSON(w, http.StatusOK, out)
}

// handleCancelJob cancels a queued job outright, or, if it's already been
// claimed and is running, requests cancellation via the Redis cancel flag
// that consumer-service polls (see internal/cancel).
func (s *JobServer) handleCancelJob(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	job, err := s.store.GetJob(r.Context(), id)
	if err != nil {
		if err == store.ErrNotFound {
			notFound(w, "job not found")
			return
		}
		internalError(w, err)
		return
	}
	switch job.Status {
	case "success", "failed", "dead", "cancelled":
		conflict(w, "job already finished")
		return
	}

	cancelled, err := s.store.CancelQueuedJob(r.Context(), id)
	if err != nil {
		internalError(w, err)
		return
	}
	if cancelled {
		writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
		return
	}

	// Already claimed/running: signal via Redis, consumer-service polls and
	// aborts the in-flight execution.
	if err := cancel.Request(r.Context(), s.redis, id); err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "cancellation_requested"})
}

func (s *JobServer) handleGetJob(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	job, err := s.store.GetJob(r.Context(), id)
	if err != nil {
		if err == store.ErrNotFound {
			notFound(w, "job not found")
			return
		}
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toJobResponse(job))
}
