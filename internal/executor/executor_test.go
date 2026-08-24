package executor

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestRunSuccess(t *testing.T) {
	out, err := Run(context.Background(), json.RawMessage(`{"cmd":"echo","args":["hello"]}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "hello") {
		t.Fatalf("expected output to contain hello, got %q", out)
	}
}

func TestRunNonZeroExit(t *testing.T) {
	_, err := Run(context.Background(), json.RawMessage(`{"cmd":"sh","args":["-c","exit 1"]}`))
	if err == nil {
		t.Fatal("expected error for non-zero exit")
	}
}

func TestRunMissingCmd(t *testing.T) {
	_, err := Run(context.Background(), json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error for missing cmd")
	}
}

func TestRunInvalidPayload(t *testing.T) {
	_, err := Run(context.Background(), json.RawMessage(`not-json`))
	if err == nil {
		t.Fatal("expected error for invalid payload")
	}
}

func TestRunTimeout(t *testing.T) {
	start := time.Now()
	_, err := Run(context.Background(), json.RawMessage(`{"cmd":"sleep","args":["5"],"timeout_seconds":1}`))
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("expected timeout to fire around 1s, took %s", elapsed)
	}
}
