import { useEffect, useState } from "react";
import * as api from "../api";
import { getSettings } from "../settings";
import LineChart from "./Chart";

// Same tokens the .status-* badge classes use (index.css) - keeps the
// donut/legend colors in sync with the badges and with light/dark theme
// switches, since CSS custom properties resolve live in inline styles too.
const STATUS_COLORS = {
  queued: "var(--status-queued)",
  running: "var(--status-running)",
  success: "var(--status-success)",
  failed: "var(--status-failed)",
  dead: "var(--status-dead)",
  cancelled: "var(--status-cancelled)",
};

function Tile({ label, value, accent }) {
  return (
    <div className={`card stat-tile ${accent ? "stat-tile-accent" : ""}`}>
      <div className="stat-value">{value}</div>
      <div className="stat-label">{label}</div>
    </div>
  );
}

function loadStatus(load, cores) {
  const ratio = cores ? load / cores : load;
  if (ratio < 0.6) return { label: "Healthy", cls: "pill-good" };
  if (ratio < 1) return { label: "Busy", cls: "pill-warn" };
  return { label: "Overloaded", cls: "pill-bad" };
}

export default function Overview() {
  const [m, setM] = useState(null);
  const [error, setError] = useState("");
  const [cpuHistory, setCpuHistory] = useState([]);

  async function refresh() {
    try {
      const data = await api.getMetrics();
      setM(data);
      setError("");
      setCpuHistory((h) => [...h, data.cpu_load_1m].slice(-40));
    } catch (err) {
      setError(err.message);
    }
  }

  useEffect(() => {
    refresh();
    const t = setInterval(refresh, getSettings().pollMs);
    return () => clearInterval(t);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  if (error && !m) return <div className="error">{error}</div>;
  if (!m) return <p className="muted">Loading…</p>;

  const totalJobs = m.jobs_queued + m.jobs_running + m.jobs_success + m.jobs_failed + m.jobs_dead + m.jobs_cancelled;
  const statusRows = [
    ["queued", m.jobs_queued],
    ["running", m.jobs_running],
    ["success", m.jobs_success],
    ["failed", m.jobs_failed],
    ["dead", m.jobs_dead],
    ["cancelled", m.jobs_cancelled],
  ];

  let acc = 0;
  const gradientStops = statusRows.map(([label, value]) => {
    const pct = totalJobs ? (value / totalJobs) * 100 : 0;
    const stop = `${STATUS_COLORS[label]} ${acc}% ${acc + pct}%`;
    acc += pct;
    return stop;
  });
  const donutStyle = { background: totalJobs ? `conic-gradient(${gradientStops.join(", ")})` : "var(--border)" };

  const status = loadStatus(m.cpu_load_1m, m.cpu_cores || 1);

  return (
    <div>
      <div className="content-header"><h2>Overview</h2></div>

      {error && <div className="error">{error}</div>}

      <div className="stat-grid">
        <Tile label="Jobs queued" value={m.jobs_queued} />
        <Tile label="Jobs running" value={m.jobs_running} accent />
        <Tile label="Completed (last hour)" value={m.completed_last_hour} />
        <Tile label="Dead-lettered" value={m.jobs_dead} />
        <Tile label="Active workers" value={`${m.workers_active} / ${m.workers}`} />
        <Tile label="Paused queues" value={`${m.queues_paused} / ${m.queues}`} />
        <Tile label="DLQ entries" value={m.dlq_entries} />
        <Tile label="Total jobs" value={totalJobs} />
      </div>

      <div className="overview-grid">
        <div className="card panel">
          <div className="panel-header">
            <h3>Server CPU load</h3>
            <span className={`pill ${status.cls}`}>{status.label}</span>
          </div>
          <LineChart data={cpuHistory} color="var(--accent)" />
          <div className="panel-footer">
            <span>{m.cpu_load_1m.toFixed(2)} load avg (1m)</span>
            <span className="muted">{m.cpu_cores} cores · 5m {m.cpu_load_5m.toFixed(2)} · 15m {m.cpu_load_15m.toFixed(2)}</span>
          </div>
        </div>

        <div className="card panel">
          <div className="panel-header"><h3>Job status</h3></div>
          <div className="donut-row">
            <div className="donut" style={donutStyle}>
              <div className="donut-hole">
                <div className="donut-total">{totalJobs}</div>
                <div className="muted">jobs</div>
              </div>
            </div>
            <div className="donut-legend">
              {statusRows.map(([label, value]) => (
                <div className="legend-row" key={label}>
                  <span className="legend-dot" style={{ background: STATUS_COLORS[label] }} />
                  <span className="legend-label">{label}</span>
                  <span className="legend-value">{value}</span>
                </div>
              ))}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
