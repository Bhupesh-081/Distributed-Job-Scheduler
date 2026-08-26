import { useEffect, useRef, useState } from "react";
import * as api from "../api";
import { fmtDate, statusClass } from "../format";
import { getSettings } from "../settings";
import JobForm from "./JobForm";

const TERMINAL = new Set(["success", "failed", "dead", "cancelled"]);

// Applies one job_updated push to the currently-filtered list: update in
// place, insert if new, or drop it if it no longer matches the status
// filter (e.g. it just left "queued").
function mergeJob(list, job, statusFilter) {
  const idx = list.findIndex((j) => j.id === job.id);
  const matches = !statusFilter || job.status === statusFilter;
  if (!matches) return idx === -1 ? list : list.filter((j) => j.id !== job.id);
  if (idx === -1) return [job, ...list];
  const next = [...list];
  next[idx] = job;
  return next;
}

export default function Jobs({ queueId, retryPolicies }) {
  const [jobs, setJobs] = useState([]);
  const [status, setStatus] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [detail, setDetail] = useState(null);
  const [logs, setLogs] = useState([]);
  const [live, setLive] = useState(false);
  const statusRef = useRef(status);
  statusRef.current = status;

  async function refresh() {
    setError("");
    try {
      setJobs(await api.listJobs(queueId, { status }));
    } catch (err) {
      setError(err.message);
    }
  }

  // Live updates over WebSocket, with a REST resync as a safety net (covers
  // the reconnect gap, and any edge case the merge logic gets wrong) - see
  // GET /jobs/stream (internal/httpapi/jobs_stream.go).
  useEffect(() => {
    refresh();
    let stopped = false;
    let ws;

    function connect() {
      if (stopped) return;
      ws = api.openJobsStream(queueId, {
        onOpen: () => setLive(true),
        onJob: (job) => setJobs((list) => mergeJob(list, job, statusRef.current)),
        onClose: () => {
          setLive(false);
          if (!stopped) setTimeout(connect, 2000);
        },
      });
    }
    connect();

    const resync = setInterval(refresh, getSettings().pollMs);
    return () => {
      stopped = true;
      clearInterval(resync);
      ws?.close();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [queueId]);

  useEffect(() => {
    refresh();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [status]);

  async function createFromForm(body) {
    setBusy(true);
    setError("");
    try {
      await api.createJob({ ...body, queue_id: queueId });
      refresh();
      return true;
    } catch (err) {
      setError(err.message);
      return false;
    } finally {
      setBusy(false);
    }
  }

  async function openJob(id) {
    setError("");
    try {
      const [job, jobLogs] = await Promise.all([api.getJob(id), api.getJobLogs(id)]);
      setDetail(job);
      setLogs(jobLogs);
    } catch (err) {
      setError(err.message);
    }
  }

  async function cancel(id) {
    setError("");
    try {
      await api.cancelJob(id);
      refresh();
      if (detail?.id === id) openJob(id);
    } catch (err) {
      setError(err.message);
    }
  }

  return (
    <div>
      <JobForm retryPolicies={retryPolicies} busy={busy} onSubmit={createFromForm} />

      {error && <div className="error">{error}</div>}

      <div className="content-header">
        <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
          <h3 style={{ margin: 0 }}>Jobs</h3>
          <span className={`pill ${live ? "pill-good" : "pill-warn"}`}>{live ? "Live" : "Reconnecting…"}</span>
        </div>
        <select value={status} onChange={(e) => setStatus(e.target.value)}>
          <option value="">All statuses</option>
          {["queued", "running", "success", "failed", "dead", "cancelled"].map((s) => (
            <option key={s} value={s}>{s}</option>
          ))}
        </select>
      </div>

      <table className="table">
        <thead>
          <tr>
            <th>Name</th><th>Type</th><th>Status</th><th>Retries</th><th>Created</th><th></th>
          </tr>
        </thead>
        <tbody>
          {jobs.map((j) => (
            <tr key={j.id}>
              <td>{j.name}</td>
              <td>{j.scheduled_type}</td>
              <td><span className={statusClass(j.status)}>{j.status}</span></td>
              <td>{j.retries_count}/{j.retries_max}</td>
              <td>{fmtDate(j.created_at)}</td>
              <td className="row-actions">
                <button className="link" onClick={() => openJob(j.id)}>Details</button>
                {!TERMINAL.has(j.status) && (
                  <button className="link danger" onClick={() => cancel(j.id)}>Cancel</button>
                )}
              </td>
            </tr>
          ))}
          {jobs.length === 0 && (
            <tr><td colSpan={6} className="muted">No jobs for this queue.</td></tr>
          )}
        </tbody>
      </table>

      {detail && (
        <div className="card" style={{ padding: 16, marginTop: 16 }}>
          <div className="content-header">
            <h3 style={{ margin: 0 }}>{detail.name}</h3>
            <button className="link" onClick={() => setDetail(null)}>Close</button>
          </div>
          <p className="muted">
            <span className={statusClass(detail.status)}>{detail.status}</span> · {detail.retries_count}/{detail.retries_max} retries · created {fmtDate(detail.created_at)}
          </p>
          <pre className="payload-view">{JSON.stringify(detail.payload, null, 2)}</pre>
          <h4>Execution log</h4>
          <table className="table">
            <thead><tr><th>Time</th><th>Level</th><th>Message</th></tr></thead>
            <tbody>
              {logs.map((l, i) => (
                <tr key={i}>
                  <td>{fmtDate(l.created_at)}</td>
                  <td>{l.level}</td>
                  <td>{l.message}</td>
                </tr>
              ))}
              {logs.length === 0 && <tr><td colSpan={3} className="muted">No log entries yet.</td></tr>}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
