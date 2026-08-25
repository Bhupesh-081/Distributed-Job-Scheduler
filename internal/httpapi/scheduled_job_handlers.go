package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"distributed-job-scheduler/internal/cronexpr"
	"distributed-job-scheduler/internal/store"
)

type scheduledJobResponse struct {
	ID             string          `json:"id"`
	QueueID        string          `json:"queue_id"`
	Name           string          `json:"name"`
	CronExpression string          `json:"cron_expression"`
	Payload        json.RawMessage `json:"payload"`
	RetriesMax     int             `json:"retries_max"`
	RetryPolicyID  *string         `json:"retry_policy_id,omitempty"`
	Active         bool            `json:"active"`
	NextRunAt      string          `json:"next_run_at"`
	LastRunAt      *string         `json:"last_run_at,omitempty"`
	CreatedAt      string          `json:"created_at"`
	UpdatedAt      string          `json:"updated_at"`
}

func toScheduledJobResponse(sj store.ScheduledJob) scheduledJobResponse {
	out := scheduledJobResponse{
		ID:             sj.ID.String(),
		QueueID:        sj.QueueID.String(),
		Name:           sj.Name,
		CronExpression: sj.CronExpression,
		Payload:        sj.Payload,
		RetriesMax:     sj.RetriesMax,
		Active:         sj.Active,
		NextRunAt:      sj.NextRunAt.Format(timeFormat),
		CreatedAt:      sj.CreatedAt.Format(timeFormat),
		UpdatedAt:      sj.UpdatedAt.Format(timeFormat),
	}
	if sj.RetryPolicyID != nil {
		s := sj.RetryPolicyID.String()
		out.RetryPolicyID = &s
	}
	if sj.LastRunAt != nil {
		s := sj.LastRunAt.Format(timeFormat)
		out.LastRunAt = &s
	}
	return out
}

// requireScheduledJobMember loads the scheduled job and confirms the
// caller has access to its owning queue.
func (s *Server) requireScheduledJobMember(w http.ResponseWriter, r *http.Request, id uuid.UUID) (store.ScheduledJob, bool) {
	sj, err := s.store.GetScheduledJob(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			notFound(w, "scheduled job not found")
			return store.ScheduledJob{}, false
		}
		internalError(w, err)
		return store.ScheduledJob{}, false
	}
	if _, ok := s.requireQueueMember(w, r, sj.QueueID); !ok {
		return store.ScheduledJob{}, false
	}
	return sj, true
}

type scheduledJobRequest struct {
	Name           string          `json:"name"`
	CronExpression string          `json:"cron_expression"`
	Payload        json.RawMessage `json:"payload"`
	RetriesMax     *int            `json:"retries_max,omitempty"`
	RetryPolicyID  string          `json:"retry_policy_id,omitempty"`
}

// validate checks everything except retry_policy_id's existence (an FK,
// left to the DB) and returns the parsed retriesMax/retryPolicyID.
func (req scheduledJobRequest) validate() (retriesMax int, retryPolicyID *uuid.UUID, err error) {
	if strings.TrimSpace(req.Name) == "" {
		return 0, nil, errors.New("name is required")
	}
	if err := cronexpr.Validate(req.CronExpression); err != nil {
		return 0, nil, errors.New("cron_expression is invalid: " + err.Error())
	}
	if err := validatePayload(req.Payload); err != nil {
		return 0, nil, err
	}
	retriesMax = 3
	if req.RetriesMax != nil {
		if *req.RetriesMax < 0 {
			return 0, nil, errors.New("retries_max must be >= 0")
		}
		retriesMax = *req.RetriesMax
	}
	if req.RetryPolicyID != "" {
		id, err := uuid.Parse(req.RetryPolicyID)
		if err != nil {
			return 0, nil, errors.New("retry_policy_id must be a valid UUID")
		}
		retryPolicyID = &id
	}
	return retriesMax, retryPolicyID, nil
}

func (s *Server) handleCreateScheduledJob(w http.ResponseWriter, r *http.Request) {
	queueID, ok := pathUUID(w, r, "queueId")
	if !ok {
		return
	}
	queue, ok := s.requireQueueMember(w, r, queueID)
	if !ok {
		return
	}

	var req scheduledJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}
	retriesMax, retryPolicyID, err := req.validate()
	if err != nil {
		badRequest(w, err.Error())
		return
	}
	nextRunAt, _ := cronexpr.Next(req.CronExpression, time.Now()) // already validated above

	sj, err := s.store.CreateScheduledJob(r.Context(), store.NewScheduledJob{
		QueueID: queue.ID, Name: strings.TrimSpace(req.Name), CronExpression: req.CronExpression,
		Payload: req.Payload, RetriesMax: retriesMax, RetryPolicyID: retryPolicyID, NextRunAt: nextRunAt,
	})
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			conflict(w, "a scheduled job with this name already exists in the queue")
			return
		}
		if errors.Is(err, store.ErrRetryPolicyNotFound) {
			badRequest(w, "retry policy not found")
			return
		}
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toScheduledJobResponse(sj))
}

func (s *Server) handleListScheduledJobs(w http.ResponseWriter, r *http.Request) {
	queueID, ok := pathUUID(w, r, "queueId")
	if !ok {
		return
	}
	if _, ok := s.requireQueueMember(w, r, queueID); !ok {
		return
	}

	limit, offset := pageParams(r)
	jobs, err := s.store.ListScheduledJobsForQueue(r.Context(), queueID, limit, offset)
	if err != nil {
		internalError(w, err)
		return
	}
	out := make([]scheduledJobResponse, len(jobs))
	for i, sj := range jobs {
		out[i] = toScheduledJobResponse(sj)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleGetScheduledJob(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "scheduledJobId")
	if !ok {
		return
	}
	sj, ok := s.requireScheduledJobMember(w, r, id)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, toScheduledJobResponse(sj))
}

func (s *Server) handleUpdateScheduledJob(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "scheduledJobId")
	if !ok {
		return
	}
	if _, ok := s.requireScheduledJobMember(w, r, id); !ok {
		return
	}

	var req scheduledJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}
	retriesMax, retryPolicyID, err := req.validate()
	if err != nil {
		badRequest(w, err.Error())
		return
	}
	// cron_expression may have changed, so the next firing is recomputed
	// from now rather than kept from before.
	nextRunAt, _ := cronexpr.Next(req.CronExpression, time.Now())

	sj, err := s.store.UpdateScheduledJob(r.Context(), id, strings.TrimSpace(req.Name), req.CronExpression, req.Payload, retriesMax, retryPolicyID, nextRunAt)
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			conflict(w, "a scheduled job with this name already exists in the queue")
			return
		}
		if errors.Is(err, store.ErrRetryPolicyNotFound) {
			badRequest(w, "retry policy not found")
			return
		}
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toScheduledJobResponse(sj))
}

func (s *Server) handleDeleteScheduledJob(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "scheduledJobId")
	if !ok {
		return
	}
	if _, ok := s.requireScheduledJobMember(w, r, id); !ok {
		return
	}
	if err := s.store.DeleteScheduledJob(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			notFound(w, "scheduled job not found")
			return
		}
		internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handlePauseScheduledJob(w http.ResponseWriter, r *http.Request) {
	s.setScheduledJobActive(w, r, false)
}

func (s *Server) handleResumeScheduledJob(w http.ResponseWriter, r *http.Request) {
	s.setScheduledJobActive(w, r, true)
}

func (s *Server) setScheduledJobActive(w http.ResponseWriter, r *http.Request, active bool) {
	id, ok := pathUUID(w, r, "scheduledJobId")
	if !ok {
		return
	}
	if _, ok := s.requireScheduledJobMember(w, r, id); !ok {
		return
	}
	sj, err := s.store.SetScheduledJobActive(r.Context(), id, active)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toScheduledJobResponse(sj))
}
