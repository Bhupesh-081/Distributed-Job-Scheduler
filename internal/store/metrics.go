package store

import "context"

// SystemMetrics is a global snapshot for the dashboard overview - not
// org/project-scoped, same convention as ListWorkers (shared infra, any
// authenticated user).
type SystemMetrics struct {
	JobsQueued        int
	JobsRunning       int
	JobsSuccess       int
	JobsFailed        int
	JobsDead          int
	JobsCancelled     int
	CompletedLastHour int
	Queues            int
	QueuesPaused      int
	Workers           int
	WorkersActive     int
	DLQEntries        int
}

func (s *Store) GetSystemMetrics(ctx context.Context) (SystemMetrics, error) {
	var m SystemMetrics
	err := s.pool.QueryRow(ctx,
		`SELECT
		   count(*) FILTER (WHERE status = 'queued'),
		   count(*) FILTER (WHERE status = 'running'),
		   count(*) FILTER (WHERE status = 'success'),
		   count(*) FILTER (WHERE status = 'failed'),
		   count(*) FILTER (WHERE status = 'dead'),
		   count(*) FILTER (WHERE status = 'cancelled'),
		   count(*) FILTER (WHERE status = 'success' AND modified_time > now() - interval '1 hour')
		 FROM jobs`,
	).Scan(&m.JobsQueued, &m.JobsRunning, &m.JobsSuccess, &m.JobsFailed, &m.JobsDead, &m.JobsCancelled, &m.CompletedLastHour)
	if err != nil {
		return m, err
	}

	if err := s.pool.QueryRow(ctx,
		`SELECT count(*), count(*) FILTER (WHERE paused) FROM queues`,
	).Scan(&m.Queues, &m.QueuesPaused); err != nil {
		return m, err
	}

	if err := s.pool.QueryRow(ctx,
		`SELECT count(*), count(*) FILTER (WHERE status = 'active') FROM workers`,
	).Scan(&m.Workers, &m.WorkersActive); err != nil {
		return m, err
	}

	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM dead_letter_queue`).Scan(&m.DLQEntries); err != nil {
		return m, err
	}

	return m, nil
}
