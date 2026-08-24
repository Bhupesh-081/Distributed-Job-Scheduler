package store

import (
	"context"
	"encoding/json"
	"slices"
	"testing"
	"time"
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
