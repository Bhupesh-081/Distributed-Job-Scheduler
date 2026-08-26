package httpapi

import (
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
)

type systemMetricsResponse struct {
	JobsQueued        int     `json:"jobs_queued"`
	JobsRunning       int     `json:"jobs_running"`
	JobsSuccess       int     `json:"jobs_success"`
	JobsFailed        int     `json:"jobs_failed"`
	JobsDead          int     `json:"jobs_dead"`
	JobsCancelled     int     `json:"jobs_cancelled"`
	CompletedLastHour int     `json:"completed_last_hour"`
	Queues            int     `json:"queues"`
	QueuesPaused      int     `json:"queues_paused"`
	Workers           int     `json:"workers"`
	WorkersActive     int     `json:"workers_active"`
	DLQEntries        int     `json:"dlq_entries"`
	CPULoad1m         float64 `json:"cpu_load_1m"`
	CPULoad5m         float64 `json:"cpu_load_5m"`
	CPULoad15m        float64 `json:"cpu_load_15m"`
	CPUCores          int     `json:"cpu_cores"`
}

// loadAvg reads the host's 1/5/15-minute load average from /proc/loadavg.
// ponytail: Linux-only (fine - every service here runs in a Linux
// container); returns zeros on any other OS or read failure instead of
// erroring, since this is a dashboard nice-to-have, not core data.
func loadAvg() (m1, m5, m15 float64) {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0, 0, 0
	}
	fields := strings.Fields(string(data))
	if len(fields) < 3 {
		return 0, 0, 0
	}
	m1, _ = strconv.ParseFloat(fields[0], 64)
	m5, _ = strconv.ParseFloat(fields[1], 64)
	m15, _ = strconv.ParseFloat(fields[2], 64)
	return m1, m5, m15
}

func (s *Server) handleSystemMetrics(w http.ResponseWriter, r *http.Request) {
	m, err := s.store.GetSystemMetrics(r.Context())
	if err != nil {
		internalError(w, err)
		return
	}
	m1, m5, m15 := loadAvg()
	writeJSON(w, http.StatusOK, systemMetricsResponse{
		JobsQueued:        m.JobsQueued,
		JobsRunning:       m.JobsRunning,
		JobsSuccess:       m.JobsSuccess,
		JobsFailed:        m.JobsFailed,
		JobsDead:          m.JobsDead,
		JobsCancelled:     m.JobsCancelled,
		CompletedLastHour: m.CompletedLastHour,
		Queues:            m.Queues,
		QueuesPaused:      m.QueuesPaused,
		Workers:           m.Workers,
		WorkersActive:     m.WorkersActive,
		DLQEntries:        m.DLQEntries,
		CPULoad1m:         m1,
		CPULoad5m:         m5,
		CPULoad15m:        m15,
		CPUCores:          runtime.NumCPU(),
	})
}
