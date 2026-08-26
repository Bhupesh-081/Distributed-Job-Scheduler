// Package cronexpr parses standard 5-field cron expressions (minute hour
// day-of-month month day-of-week, e.g. "*/5 * * * *") and computes the next
// occurrence after a given time, the only two operations scheduled_jobs
// needs from a cron library, so this wraps robfig/cron/v3's parser rather
// than pulling in its scheduler (watcher-service already has its own tick
// loop).
package cronexpr

import (
	"time"

	"github.com/robfig/cron/v3"
)

var parser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

// Validate returns an error if expr isn't a valid standard cron expression.
func Validate(expr string) error {
	_, err := parser.Parse(expr)
	return err
}

// Next returns the next time expr fires strictly after `after`.
func Next(expr string, after time.Time) (time.Time, error) {
	sched, err := parser.Parse(expr)
	if err != nil {
		return time.Time{}, err
	}
	return sched.Next(after), nil
}
