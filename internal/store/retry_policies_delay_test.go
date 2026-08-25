package store

import "testing"

func TestRetryPolicyDelay(t *testing.T) {
	cap5 := 5
	cases := []struct {
		name    string
		policy  RetryPolicy
		attempt int
		want    int // seconds
	}{
		{"fixed", RetryPolicy{Strategy: "fixed", BaseDelaySeconds: 10}, 1, 10},
		{"fixed ignores attempt", RetryPolicy{Strategy: "fixed", BaseDelaySeconds: 10}, 5, 10},
		{"linear attempt 1", RetryPolicy{Strategy: "linear", BaseDelaySeconds: 3}, 1, 3},
		{"linear attempt 3", RetryPolicy{Strategy: "linear", BaseDelaySeconds: 3}, 3, 9},
		{"exponential attempt 1", RetryPolicy{Strategy: "exponential", BaseDelaySeconds: 2}, 1, 2},
		{"exponential attempt 3", RetryPolicy{Strategy: "exponential", BaseDelaySeconds: 2}, 3, 8},
		{"exponential attempt 4", RetryPolicy{Strategy: "exponential", BaseDelaySeconds: 2}, 4, 16},
		{"max_delay caps linear", RetryPolicy{Strategy: "linear", BaseDelaySeconds: 3, MaxDelaySeconds: &cap5}, 3, 5},
		{"max_delay caps exponential", RetryPolicy{Strategy: "exponential", BaseDelaySeconds: 2, MaxDelaySeconds: &cap5}, 4, 5},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := c.policy.Delay(c.attempt)
			if got.Seconds() != float64(c.want) {
				t.Fatalf("got %v, want %ds", got, c.want)
			}
		})
	}
}
