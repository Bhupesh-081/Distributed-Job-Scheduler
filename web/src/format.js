export function fmtDate(s) {
  return s ? new Date(s).toLocaleString() : "-";
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
