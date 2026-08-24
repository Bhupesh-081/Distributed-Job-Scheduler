package httpapi

import (
	"encoding/json"
	"testing"
	"time"
)

func TestParseNewJobImmediate(t *testing.T) {
	now := time.Now()
	job, err := parseNewJob(createJobRequest{
		Name:          "send-email",
		ScheduledType: "immediate",
		Payload:       json.RawMessage(`{"cmd":"echo"}`),
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if job.ScheduledTime == nil || !job.ScheduledTime.Equal(now) {
		t.Fatalf("expected scheduled_time == now, got %v", job.ScheduledTime)
	}
	if job.RetriesMax != 3 {
		t.Fatalf("expected default retries_max 3, got %d", job.RetriesMax)
	}
}

func TestParseNewJobDelayed(t *testing.T) {
	now := time.Now()
	delay := 30
	job, err := parseNewJob(createJobRequest{
		Name:          "reminder",
		ScheduledType: "delayed",
		DelaySeconds:  &delay,
		Payload:       json.RawMessage(`{}`),
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	want := now.Add(30 * time.Second)
	if !job.ScheduledTime.Equal(want) {
		t.Fatalf("got %v, want %v", job.ScheduledTime, want)
	}

	if _, err := parseNewJob(createJobRequest{
		Name: "bad", ScheduledType: "delayed", Payload: json.RawMessage(`{}`),
	}, now); err == nil {
		t.Fatal("expected error for missing delay_seconds")
	}
}

func TestParseNewJobScheduled(t *testing.T) {
	now := time.Now()
	future := now.Add(time.Hour).Format(time.RFC3339)
	job, err := parseNewJob(createJobRequest{
		Name: "future", ScheduledType: "scheduled", ScheduledTime: &future, Payload: json.RawMessage(`{}`),
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if !job.ScheduledTime.After(now) {
		t.Fatalf("expected future scheduled_time, got %v", job.ScheduledTime)
	}

	past := now.Add(-time.Hour).Format(time.RFC3339)
	if _, err := parseNewJob(createJobRequest{
		Name: "bad", ScheduledType: "scheduled", ScheduledTime: &past, Payload: json.RawMessage(`{}`),
	}, now); err == nil {
		t.Fatal("expected error for past scheduled_time")
	}
}

func TestParseNewJobValidation(t *testing.T) {
	now := time.Now()
	cases := []createJobRequest{
		{ScheduledType: "immediate", Payload: json.RawMessage(`{}`)},                  // missing name
		{Name: "x", ScheduledType: "immediate"},                                       // missing payload
		{Name: "x", ScheduledType: "immediate", Payload: json.RawMessage(`not-json`)}, // invalid payload
		{Name: "x", ScheduledType: "recurring", Payload: json.RawMessage(`{}`)},       // not supported yet
		{Name: "x", ScheduledType: "bogus", Payload: json.RawMessage(`{}`)},           // unknown type
	}
	for i, c := range cases {
		if _, err := parseNewJob(c, now); err == nil {
			t.Fatalf("case %d: expected error, got none", i)
		}
	}
}
