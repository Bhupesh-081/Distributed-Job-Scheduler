package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

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
	QueueID       string          `json:"queue_id"`
	RetryPolicyID string          `json:"retry_policy_id,omitempty"` // overrides the queue's default_retry_policy_id
}

// validatePayload is shared between job creation and scheduled-job
// (cron template) creation — both need a non-empty, valid-JSON payload.
func validatePayload(payload json.RawMessage) error {
	if len(payload) == 0 || string(payload) == "null" {
		return fmt.Errorf("payload is required")
	}
	if !json.Valid(payload) {
		return fmt.Errorf("payload must be valid JSON")
	}
	return nil
}

// parseNewJob validates a createJobRequest and turns it into a store.NewJob.
// Pure (takes "now" as input) so it's testable without a database.
func parseNewJob(req createJobRequest, now time.Time) (store.NewJob, error) {
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return store.NewJob{}, fmt.Errorf("name is required")
	}
	if err := validatePayload(req.Payload); err != nil {
		return store.NewJob{}, err
	}
	queueID, err := uuid.Parse(req.QueueID)
	if err != nil {
		return store.NewJob{}, fmt.Errorf("queue_id must be a valid queue UUID")
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
		QueueID:       &queueID,
	}
	if req.RetryPolicyID != "" {
		id, err := uuid.Parse(req.RetryPolicyID)
		if err != nil {
			return store.NewJob{}, fmt.Errorf("retry_policy_id must be a valid UUID")
		}
		out.RetryPolicyID = &id
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
		return store.NewJob{}, fmt.Errorf("recurring jobs are created via POST /queues/{queueId}/scheduled-jobs, not POST /jobs")
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
	QueueID        *string         `json:"queue_id,omitempty"`
	RetryPolicyID  *string         `json:"retry_policy_id,omitempty"`
	ScheduledJobID *string         `json:"scheduled_job_id,omitempty"`
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
	if j.QueueID != nil {
		s := j.QueueID.String()
		out.QueueID = &s
	}
	if j.RetryPolicyID != nil {
		s := j.RetryPolicyID.String()
		out.RetryPolicyID = &s
	}
	if j.ScheduledJobID != nil {
		s := j.ScheduledJobID.String()
		out.ScheduledJobID = &s
	}
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
	if _, ok := requireQueueAccess(w, r, s.store, *newJob.QueueID); !ok {
		return
	}
	job, err := s.store.CreateJob(r.Context(), newJob)
	if err != nil {
		if err == store.ErrQueueNotFound {
			badRequest(w, "queue not found")
			return
		}
		if err == store.ErrRetryPolicyNotFound {
			badRequest(w, "retry policy not found")
			return
		}
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
	checkedQueues := make(map[uuid.UUID]bool)
	for i, jr := range req.Jobs {
		nj, err := parseNewJob(jr, now)
		if err != nil {
			badRequest(w, fmt.Sprintf("jobs[%d]: %s", i, err.Error()))
			return
		}
		if !checkedQueues[*nj.QueueID] {
			if _, ok := requireQueueAccess(w, r, s.store, *nj.QueueID); !ok {
				return
			}
			checkedQueues[*nj.QueueID] = true
		}
		newJobs[i] = nj
	}

	jobs, err := s.store.CreateJobsBatch(r.Context(), newJobs)
	if err != nil {
		if err == store.ErrQueueNotFound {
			badRequest(w, "queue not found")
			return
		}
		if err == store.ErrRetryPolicyNotFound {
			badRequest(w, "retry policy not found")
			return
		}
		internalError(w, err)
		return
	}
	out := make([]jobResponse, len(jobs))
	for i, j := range jobs {
		out[i] = toJobResponse(j)
	}
	writeJSON(w, http.StatusCreated, out)
}

// handleListJobs requires queue_id (rather than optionally filtering by it)
// now that jobs are auth-scoped: without it there'd be no single queue to
// check membership against, and "every job across every org" was never a
// meaningful listing in a multi-tenant system anyway.
func (s *JobServer) handleListJobs(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	queueID, err := uuid.Parse(r.URL.Query().Get("queue_id"))
	if err != nil {
		badRequest(w, "queue_id is required and must be a valid UUID")
		return
	}
	if _, ok := requireQueueAccess(w, r, s.store, queueID); !ok {
		return
	}
	limit, offset := pageParams(r)
	jobs, err := s.store.ListJobs(r.Context(), status, &queueID, limit, offset)
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

// requireJobAccess loads a job and confirms the caller has access to its
// queue. A job with no queue_id (an unscoped legacy job, predating
// queue-scoped auth) has no owning org, so it 404s rather than being
// exposed to any authenticated user — same call as DLQ entries.
func requireJobAccess(w http.ResponseWriter, r *http.Request, st *store.Store, id uuid.UUID) (store.Job, bool) {
	job, err := st.GetJob(r.Context(), id)
	if err != nil {
		if err == store.ErrNotFound {
			notFound(w, "job not found")
			return store.Job{}, false
		}
		internalError(w, err)
		return store.Job{}, false
	}
	if job.QueueID == nil {
		notFound(w, "job not found")
		return store.Job{}, false
	}
	if _, ok := requireQueueAccess(w, r, st, *job.QueueID); !ok {
		return store.Job{}, false
	}
	return job, true
}

// handleCancelJob cancels a queued job outright, or, if it's already been
// claimed and is running, requests cancellation via the Redis cancel flag
// that consumer-service polls (see internal/cancel).
func (s *JobServer) handleCancelJob(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	job, ok := requireJobAccess(w, r, s.store, id)
	if !ok {
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
	job, ok := requireJobAccess(w, r, s.store, id)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, toJobResponse(job))
}

type jobLogResponse struct {
	Level     string  `json:"level"`
	Message   string  `json:"message"`
	CreatedAt string  `json:"created_at"`
	JobRunID  *string `json:"job_run_id,omitempty"`
}

func (s *JobServer) handleGetJobLogs(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	if _, ok := requireJobAccess(w, r, s.store, id); !ok {
		return
	}
	limit, offset := pageParams(r)
	logs, err := s.store.ListJobLogs(r.Context(), id, limit, offset)
	if err != nil {
		internalError(w, err)
		return
	}
	out := make([]jobLogResponse, len(logs))
	for i, l := range logs {
		out[i] = jobLogResponse{Level: l.Level, Message: l.Message, CreatedAt: l.CreatedAt.Format(timeFormat)}
		if l.JobRunID != nil {
			s := l.JobRunID.String()
			out[i].JobRunID = &s
		}
	}
	writeJSON(w, http.StatusOK, out)
}
