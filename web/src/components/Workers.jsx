import { useEffect, useState } from "react";
import * as api from "../api";
import { fmtDate, statusClass } from "../format";
import { getSettings } from "../settings";

export default function Workers() {
  const [workers, setWorkers] = useState([]);
  const [status, setStatus] = useState("");
  const [selected, setSelected] = useState(null);
  const [error, setError] = useState("");

  async function refresh() {
    setError("");
    try {
      setWorkers(await api.listWorkers(status));
    } catch (err) {
      setError(err.message);
    }
  }

  useEffect(() => {
    refresh();
    const t = setInterval(refresh, getSettings().pollMs);
    return () => clearInterval(t);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [status]);

  async function openWorker(id) {
    setError("");
    try {
      setSelected(await api.getWorker(id));
    } catch (err) {
      setError(err.message);
    }
  }

  return (
    <div>
      <div className="content-header">
        <h2>Workers</h2>
        <select value={status} onChange={(e) => setStatus(e.target.value)}>
          <option value="">All statuses</option>
          <option value="active">Active</option>
          <option value="stopped">Stopped</option>
        </select>
      </div>

      {error && <div className="error">{error}</div>}

      <table className="table">
        <thead>
          <tr>
            <th>Hostname</th>
            <th>PID</th>
            <th>Concurrency</th>
            <th>Status</th>
            <th>Last heartbeat</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          {workers.map((w) => (
            <tr key={w.id}>
              <td>{w.hostname}</td>
              <td>{w.pid}</td>
              <td>{w.concurrency}</td>
              <td><span className={statusClass(w.status)}>{w.status}</span></td>
              <td>{fmtDate(w.last_heartbeat_at)}</td>
              <td><button className="link" onClick={() => openWorker(w.id)}>Details</button></td>
            </tr>
          ))}
          {workers.length === 0 && (
            <tr><td colSpan={6} className="muted">No workers registered.</td></tr>
          )}
        </tbody>
      </table>

      {selected && (
        <div className="card" style={{ padding: 16, marginTop: 16 }}>
          <div className="content-header">
            <h3 style={{ margin: 0 }}>{selected.hostname} (pid {selected.pid})</h3>
            <button className="link" onClick={() => setSelected(null)}>Close</button>
          </div>
          <p className="muted">
            Started {fmtDate(selected.started_at)}
            {selected.stopped_at ? ` · Stopped ${fmtDate(selected.stopped_at)}` : ""}
          </p>
          <h4>Recent heartbeats</h4>
          <table className="table">
            <thead><tr><th>Time</th><th>In-flight jobs</th></tr></thead>
            <tbody>
              {(selected.recent_heartbeats || []).map((h, i) => (
                <tr key={i}><td>{fmtDate(h.heartbeat_at)}</td><td>{h.in_flight_count}</td></tr>
              ))}
              {(!selected.recent_heartbeats || selected.recent_heartbeats.length === 0) && (
                <tr><td colSpan={2} className="muted">No heartbeats recorded.</td></tr>
              )}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
