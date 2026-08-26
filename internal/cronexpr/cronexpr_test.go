package cronexpr

import (
	"testing"
	"time"
)

func TestValidate(t *testing.T) {
	if err := Validate("*/5 * * * *"); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}
	if err := Validate("not a cron expression"); err == nil {
		t.Fatal("expected an error for garbage input")
	}
}

func TestNext(t *testing.T) {
	// "0 * * * *": top of every hour.
	after := time.Date(2026, 1, 1, 10, 15, 0, 0, time.UTC)
	got, err := Next("0 * * * *", after)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 1, 1, 11, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("got %v, want %v", got, want)
	}

	if _, err := Next("garbage", after); err == nil {
		t.Fatal("expected an error for an invalid expression")
	}
}
