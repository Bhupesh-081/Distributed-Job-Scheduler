export function fmtDate(s) {
  return s ? new Date(s).toLocaleString() : "-";
}

const TERMINAL_STATUSES = new Set(["success", "failed", "dead", "cancelled"]);

// created_at -> modified_time as a "time in system" proxy for a terminal
// job (no per-attempt start/end time at the list level without an extra
// fetch per row - close enough for a duration column, exact figures are
// still in the job's execution log).
export function durationMs(job) {
  if (!TERMINAL_STATUSES.has(job.status)) return null;
  const ms = new Date(job.modified_time) - new Date(job.created_at);
  return Number.isFinite(ms) && ms >= 0 ? ms : null;
}

export function fmtDuration(job) {
  const ms = durationMs(job);
  if (ms === null) return "-";
  if (ms < 1000) return `${ms}ms`;
  const s = ms / 1000;
  if (s < 60) return `${s.toFixed(1)}s`;
  const m = Math.floor(s / 60);
  return `${m}m ${Math.round(s % 60)}s`;
}

const STATUS_CLASS = {
  queued: "status-queued",
  scheduled: "status-queued",
  running: "status-running",
  success: "status-success",
  failed: "status-failed",
  dead: "status-dead",
  cancelled: "status-cancelled",
  active: "status-success",
  stopped: "status-failed",
};

export function statusClass(status) {
  return `status-badge ${STATUS_CLASS[status] || ""}`;
}
