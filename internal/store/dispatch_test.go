package store

import (
	"context"
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestDispatchDueJobs(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	now := time.Now()
	past := now.Add(-time.Minute)
	future := now.Add(time.Hour)

	due, err := s.CreateJob(ctx, NewJob{Name: "due", ScheduledType: "immediate", ScheduledTime: &past, Payload: json.RawMessage(`{}`), RetriesMax: 3})
	if err != nil {
		t.Fatal(err)
	}
	notDue, err := s.CreateJob(ctx, NewJob{Name: "not-due", ScheduledType: "scheduled", ScheduledTime: &future, Payload: json.RawMessage(`{}`), RetriesMax: 3})
	if err != nil {
		t.Fatal(err)
	}

	ids, err := s.DispatchDueJobs(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(ids, due.ID) {
		t.Fatalf("expected due job %s in dispatched set %v", due.ID, ids)
	}
	if slices.Contains(ids, notDue.ID) {
		t.Fatalf("did not expect future job %s to be dispatched", notDue.ID)
	}

	// Second call must not redispatch the same job (dispatched_at now set).
	ids2, err := s.DispatchDueJobs(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(ids2, due.ID) {
		t.Fatal("expected already-dispatched job not to be dispatched again")
	}
}

func TestRecoverStuckJobs(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	past := time.Now().Add(-time.Minute)
	job, err := s.CreateJob(ctx, NewJob{Name: "stuck", ScheduledType: "immediate", ScheduledTime: &past, Payload: json.RawMessage(`{}`), RetriesMax: 3})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DispatchDueJobs(ctx, 100); err != nil {
		t.Fatal(err)
	}

	// Not stale yet: a generous threshold should find nothing.
	ids, err := s.RecoverStuckJobs(ctx, time.Hour, 100)
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(ids, job.ID) {
		t.Fatal("did not expect a freshly-dispatched job to be recovered")
	}

	// A negative threshold treats it as stale immediately, without racing
	// modified_time against "now" in this test.
	ids, err = s.RecoverStuckJobs(ctx, -time.Second, 100)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(ids, job.ID) {
		t.Fatalf("expected stuck job %s to be recovered, got %v", job.ID, ids)
	}

	got, err := s.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "queued" {
		t.Fatalf("expected recovered job to stay queued, got %s", got.Status)
	}

	// After recovery, dispatched_at is cleared, so it's due again.
	ids, err = s.DispatchDueJobs(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(ids, job.ID) {
		t.Fatal("expected recovered job to be re-dispatchable")
	}
}

// claimAndRun creates, dispatches, and claims a job, then creates its
// job_runs row — i.e. everything a real worker does up to (not including)
// actually executing the payload, the point at which a crash leaves a job
// stuck 'running'.
func claimAndRun(t *testing.T, s *Store, retriesMax int) Job {
	t.Helper()
	ctx := context.Background()
	queue := testQueue(t, s)
	past := time.Now().Add(-time.Minute)
	job, err := s.CreateJob(ctx, NewJob{Name: "crash-test", ScheduledType: "immediate", ScheduledTime: &past, Payload: json.RawMessage(`{}`), RetriesMax: retriesMax, QueueID: &queue.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DispatchDueJobs(ctx, 100); err != nil {
		t.Fatal(err)
	}
	claimed, ok, err := s.ClaimJob(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected to claim the freshly dispatched job")
	}
	worker, err := s.RegisterWorker(ctx, uuid.New(), "crash-test-host", 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateJobRun(ctx, job.ID, 1, worker.ID); err != nil {
		t.Fatal(err)
	}
	return claimed
}

func TestRecoverStuckRunningJobsRequeues(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	job := claimAndRun(t, s, 3) // retries_max=3: one crash shouldn't exhaust it

	recovered, err := s.RecoverStuckRunningJobs(ctx, -time.Second, 100)
	if err != nil {
		t.Fatal(err)
	}
	var found *RecoveredRun
	for i := range recovered {
		if recovered[i].JobID == job.ID {
			found = &recovered[i]
		}
	}
	if found == nil {
		t.Fatalf("expected job %s among recovered, got %v", job.ID, recovered)
	}
	if found.Dead {
		t.Fatal("expected requeue, not dead-letter, with retries remaining")
	}

	got, err := s.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "queued" {
		t.Fatalf("expected status=queued after recovery, got %s", got.Status)
	}
	if got.RetriesCount != 1 {
		t.Fatalf("expected retries_count=1 (crash counts as a failure), got %d", got.RetriesCount)
	}

	logs, err := s.ListJobLogs(ctx, job.ID, 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) == 0 || logs[len(logs)-1].Level != "error" {
		t.Fatalf("expected a crash job_logs entry, got %+v", logs)
	}

	// Requeued and re-dispatchable in the same tick as the recovery itself.
	ids, err := s.DispatchDueJobs(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(ids, job.ID) {
		t.Fatal("expected recovered-and-requeued job to be immediately re-dispatchable")
	}
}

func TestRecoverStuckRunningJobsDeadLetters(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	job := claimAndRun(t, s, 0) // retries_max=0: any failure exhausts it immediately

	recovered, err := s.RecoverStuckRunningJobs(ctx, -time.Second, 100)
	if err != nil {
		t.Fatal(err)
	}
	var found *RecoveredRun
	for i := range recovered {
		if recovered[i].JobID == job.ID {
			found = &recovered[i]
		}
	}
	if found == nil {
		t.Fatalf("expected job %s among recovered, got %v", job.ID, recovered)
	}
	if !found.Dead {
		t.Fatal("expected dead-letter with retries_max=0")
	}

	got, err := s.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "dead" {
		t.Fatalf("expected status=dead, got %s", got.Status)
	}

	dlq, err := s.ListDLQForQueue(ctx, *job.QueueID, 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	var inDLQ bool
	for _, e := range dlq {
		if e.JobID == job.ID {
			inDLQ = true
		}
	}
	if !inDLQ {
		t.Fatalf("expected a dead_letter_queue entry for job %s", job.ID)
	}
}
