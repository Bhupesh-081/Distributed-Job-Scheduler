package store

import (
	"context"
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
)

// testQueue creates a full org/project/queue chain so scheduled_jobs (and
// anything else needing a real queue_id) has something to reference. Names
// are randomized since these tests run against a shared, non-isolated dev
// DB (see the store integration-test gotcha in project memory).
func testQueue(t *testing.T, s *Store) Queue {
	t.Helper()
	ctx := context.Background()
	suffix := uuid.New().String()[:8]

	user, err := s.CreateUser(ctx, "scheduled-jobs-test-"+suffix+"@example.com", "hash")
	if err != nil {
		t.Fatal(err)
	}
	org, err := s.CreateOrganization(ctx, "org-"+suffix, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	project, err := s.CreateProject(ctx, org.ID, "project-"+suffix)
	if err != nil {
		t.Fatal(err)
	}
	queue, err := s.CreateQueue(ctx, NewQueue{ProjectID: project.ID, Name: "queue-" + suffix, ConcurrencyLimit: 5})
	if err != nil {
		t.Fatal(err)
	}
	return queue
}

func TestExpandDueScheduledJobs(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	queue := testQueue(t, s)

	due, err := s.CreateScheduledJob(ctx, NewScheduledJob{
		QueueID: queue.ID, Name: "due-cron", CronExpression: "* * * * *",
		Payload: json.RawMessage(`{"cmd":"true"}`), RetriesMax: 3, NextRunAt: time.Now().Add(-time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	notDue, err := s.CreateScheduledJob(ctx, NewScheduledJob{
		QueueID: queue.ID, Name: "not-due-cron", CronExpression: "* * * * *",
		Payload: json.RawMessage(`{"cmd":"true"}`), RetriesMax: 3, NextRunAt: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}

	// limit=100 and the shared, non-isolated dev DB can have other due
	// scheduled_jobs left over from earlier runs (see testQueue's comment),
	// so this looks for the job traced back to `due` among everything that
	// fired, rather than asserting an exact spawned count, same reasoning
	// TestDispatchDueJobs/TestRecoverStuckJobs already use.
	spawned, err := s.ExpandDueScheduledJobs(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	var job Job
	var foundSpawned bool
	for _, id := range spawned {
		j, err := s.GetJob(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if j.ScheduledJobID != nil && *j.ScheduledJobID == due.ID {
			job, foundSpawned = j, true
			break
		}
	}
	if !foundSpawned {
		t.Fatalf("expected a job spawned from scheduled job %s, got spawned job IDs %v", due.ID, spawned)
	}
	if job.Name != "due-cron" || job.Status != "queued" || job.ScheduledType != "immediate" {
		t.Fatalf("unexpected spawned job: %+v", job)
	}
	if job.ScheduledJobID == nil || *job.ScheduledJobID != due.ID {
		t.Fatalf("expected spawned job to trace back to %s, got %v", due.ID, job.ScheduledJobID)
	}
	if job.QueueID == nil || *job.QueueID != queue.ID {
		t.Fatalf("expected spawned job queue_id %s, got %v", queue.ID, job.QueueID)
	}

	// next_run_at must have advanced into the future, and last_run_at set.
	updated, err := s.GetScheduledJob(ctx, due.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !updated.NextRunAt.After(time.Now()) {
		t.Fatalf("expected next_run_at to advance into the future, got %v", updated.NextRunAt)
	}
	if updated.LastRunAt == nil {
		t.Fatal("expected last_run_at to be set after firing")
	}

	// Immediately calling again must not double-fire `due` (next_run_at moved on).
	spawned2, err := s.ExpandDueScheduledJobs(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(spawned2, job.ID) {
		t.Fatalf("expected no re-fire of the just-fired job, got %v", spawned2)
	}
	for _, id := range spawned2 {
		j, err := s.GetJob(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if j.ScheduledJobID != nil && *j.ScheduledJobID == due.ID {
			t.Fatalf("expected scheduled job %s not to re-fire immediately, but it spawned %s", due.ID, id)
		}
	}

	// The not-yet-due one should never have fired.
	notDueStill, err := s.GetScheduledJob(ctx, notDue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if notDueStill.LastRunAt != nil {
		t.Fatal("expected not-due scheduled job to never have fired")
	}
}

func TestScheduledJobPauseSkipsExpansion(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	queue := testQueue(t, s)

	sj, err := s.CreateScheduledJob(ctx, NewScheduledJob{
		QueueID: queue.ID, Name: "paused-cron", CronExpression: "* * * * *",
		Payload: json.RawMessage(`{}`), RetriesMax: 3, NextRunAt: time.Now().Add(-time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetScheduledJobActive(ctx, sj.ID, false); err != nil {
		t.Fatal(err)
	}

	if _, err := s.ExpandDueScheduledJobs(ctx, 100); err != nil {
		t.Fatal(err)
	}

	still, err := s.GetScheduledJob(ctx, sj.ID)
	if err != nil {
		t.Fatal(err)
	}
	if still.LastRunAt != nil {
		t.Fatal("expected a paused scheduled job not to fire")
	}
}
