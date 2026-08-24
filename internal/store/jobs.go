package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type Job struct {
	ID             uuid.UUID
	Name           string
	ScheduledType  string
	Status         string
	ScheduledTime  *time.Time
	CronExpression *string
	Payload        json.RawMessage
	RetriesCount   int
	RetriesMax     int
	ModifiedTime   time.Time
	CreatedAt      time.Time
}

type NewJob struct {
	Name           string
	ScheduledType  string
	ScheduledTime  *time.Time
	CronExpression *string
	Payload        json.RawMessage
	RetriesMax     int
}

func (s *Store) CreateJob(ctx context.Context, j NewJob) (Job, error) {
	var out Job
	err := s.pool.QueryRow(ctx,
		`INSERT INTO jobs (name, scheduled_type, scheduled_time, cron_expression, payload, retries_max)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, name, scheduled_type, status, scheduled_time, cron_expression, payload, retries_count, retries_max, modified_time, created_at`,
		j.Name, j.ScheduledType, j.ScheduledTime, j.CronExpression, j.Payload, j.RetriesMax,
	).Scan(&out.ID, &out.Name, &out.ScheduledType, &out.Status, &out.ScheduledTime, &out.CronExpression,
		&out.Payload, &out.RetriesCount, &out.RetriesMax, &out.ModifiedTime, &out.CreatedAt)
	return out, err
}

// CreateJobsBatch inserts all jobs in one transaction: all-or-nothing, so a
// batch request never leaves a partial set of jobs behind.
func (s *Store) CreateJobsBatch(ctx context.Context, jobs []NewJob) ([]Job, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	out := make([]Job, 0, len(jobs))
	for _, j := range jobs {
		var row Job
		err := tx.QueryRow(ctx,
			`INSERT INTO jobs (name, scheduled_type, scheduled_time, cron_expression, payload, retries_max)
			 VALUES ($1, $2, $3, $4, $5, $6)
			 RETURNING id, name, scheduled_type, status, scheduled_time, cron_expression, payload, retries_count, retries_max, modified_time, created_at`,
			j.Name, j.ScheduledType, j.ScheduledTime, j.CronExpression, j.Payload, j.RetriesMax,
		).Scan(&row.ID, &row.Name, &row.ScheduledType, &row.Status, &row.ScheduledTime, &row.CronExpression,
			&row.Payload, &row.RetriesCount, &row.RetriesMax, &row.ModifiedTime, &row.CreatedAt)
		if err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, tx.Commit(ctx)
}

func (s *Store) GetJob(ctx context.Context, id uuid.UUID) (Job, error) {
	var out Job
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, scheduled_type, status, scheduled_time, cron_expression, payload, retries_count, retries_max, modified_time, created_at
		 FROM jobs WHERE id = $1`,
		id,
	).Scan(&out.ID, &out.Name, &out.ScheduledType, &out.Status, &out.ScheduledTime, &out.CronExpression,
		&out.Payload, &out.RetriesCount, &out.RetriesMax, &out.ModifiedTime, &out.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, ErrNotFound
	}
	return out, err
}

// ListJobs returns jobs newest-first, optionally filtered by status.
func (s *Store) ListJobs(ctx context.Context, status string, limit, offset int) ([]Job, error) {
	var rows pgx.Rows
	var err error
	if status == "" {
		rows, err = s.pool.Query(ctx,
			`SELECT id, name, scheduled_type, status, scheduled_time, cron_expression, payload, retries_count, retries_max, modified_time, created_at
			 FROM jobs ORDER BY created_at DESC LIMIT $1 OFFSET $2`,
			limit, offset)
	} else {
		rows, err = s.pool.Query(ctx,
			`SELECT id, name, scheduled_type, status, scheduled_time, cron_expression, payload, retries_count, retries_max, modified_time, created_at
			 FROM jobs WHERE status = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
			status, limit, offset)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []Job
	for rows.Next() {
		var j Job
		if err := rows.Scan(&j.ID, &j.Name, &j.ScheduledType, &j.Status, &j.ScheduledTime, &j.CronExpression,
			&j.Payload, &j.RetriesCount, &j.RetriesMax, &j.ModifiedTime, &j.CreatedAt); err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}
