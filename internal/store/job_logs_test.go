package store

import "testing"

func TestTruncateLogMessage(t *testing.T) {
	short := "all good"
	if got := truncateLogMessage(short); got != short {
		t.Fatalf("short message should pass through unchanged, got %q", got)
	}

	long := make([]byte, maxLogMessageLen+100)
	for i := range long {
		long[i] = 'x'
	}
	got := truncateLogMessage(string(long))
	if len(got) != maxLogMessageLen+len("...(truncated)") {
		t.Fatalf("expected truncated length %d, got %d", maxLogMessageLen+len("...(truncated)"), len(got))
	}
}
