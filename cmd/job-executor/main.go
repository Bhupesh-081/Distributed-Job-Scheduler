// Command job-executor is not deployed separately for the MVP: "run a
// single job's payload" turned out small enough to be a plain function
// (internal/executor.Run) that cmd/consumer-service calls in-process
// instead of shelling out to a second binary over some new transport. This
// stub is kept as the place to split it into a real standalone worker if
// consumer-service and execution ever need to scale independently.
package main

import "log/slog"

func main() {
	slog.Info("job-executor: folded into consumer-service, see internal/executor")
}
