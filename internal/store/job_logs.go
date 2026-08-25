package store

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type JobLog struct {
	ID        uuid.UUID
	JobID     uuid.UUID
	JobRunID  *uuid.UUID
	Level     string
	Message   string
	CreatedAt time.Time
}

// maxLogMessageLen keeps a runaway command's output from bloating job_logs
// indefinitely; a job needing full output capture belongs on a real log
// sink, not this table.
const maxLogMessageLen = 4000

func truncateLogMessage(msg string) string {
	if len(msg) <= maxLogMessageLen {
		return msg
	}
	return msg[:maxLogMessageLen] + "...(truncated)"
}

func (s *Store) CreateJobLog(ctx context.Context, jobID uuid.UUID, jobRunID *uuid.UUID, level, message string) (JobLog, error) {
	var l JobLog
	err := s.pool.QueryRow(ctx,
		`INSERT INTO job_logs (job_id, job_run_id, level, message)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, job_id, job_run_id, level, message, created_at`,
		jobID, jobRunID, level, truncateLogMessage(message),
	).Scan(&l.ID, &l.JobID, &l.JobRunID, &l.Level, &l.Message, &l.CreatedAt)
	return l, err
}

// ListJobLogs returns a job's log stream oldest-first (chronological, like
// a log tail), unlike the newest-first list endpoints elsewhere.
func (s *Store) ListJobLogs(ctx context.Context, jobID uuid.UUID, limit, offset int) ([]JobLog, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, job_id, job_run_id, level, message, created_at
		 FROM job_logs WHERE job_id = $1 ORDER BY created_at ASC LIMIT $2 OFFSET $3`,
		jobID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []JobLog
	for rows.Next() {
		var l JobLog
		if err := rows.Scan(&l.ID, &l.JobID, &l.JobRunID, &l.Level, &l.Message, &l.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}
