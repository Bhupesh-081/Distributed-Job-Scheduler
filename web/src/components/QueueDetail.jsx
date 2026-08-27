import { useEffect, useState } from "react";
import * as api from "../api";
import { getSettings } from "../settings";
import Jobs from "./Jobs";
import ScheduledJobs from "./ScheduledJobs";
import Dlq from "./Dlq";

const TABS = ["Jobs", "Scheduled jobs", "Dead letter queue"];

export default function QueueDetail({ queue, retryPolicies, scripts, onChanged }) {
  const [tab, setTab] = useState("Jobs");
  const [stats, setStats] = useState(null);
  const [error, setError] = useState("");

  async function refreshStats() {
    try {
      setStats(await api.getQueueStats(queue.id));
    } catch (err) {
      setError(err.message);
    }
  }

  useEffect(() => {
    refreshStats();
    const t = setInterval(refreshStats, getSettings().pollMs);
    return () => clearInterval(t);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [queue.id]);

  async function toggle() {
    setError("");
    try {
      const updated = queue.paused ? await api.resumeQueue(queue.id) : await api.pauseQueue(queue.id);
      onChanged(updated);
    } catch (err) {
      setError(err.message);
    }
  }

  return (
    <div>
      <div className="content-header">
        <div>
          <h2 style={{ margin: 0 }}>{queue.name}</h2>
          <p className="muted">
            priority {queue.priority} · concurrency {queue.concurrency_limit} · {queue.paused ? "paused" : "active"}
          </p>
        </div>
        <button className="btn-primary" onClick={toggle}>{queue.paused ? "Resume queue" : "Pause queue"}</button>
      </div>

      {error && <div className="error">{error}</div>}

      {stats && (
        <div className="stat-grid" style={{ marginBottom: 20 }}>
          {Object.entries(stats).map(([k, v]) => (
            <div className="card stat-tile" key={k}>
              <div className="stat-value">{v}</div>
              <div className="stat-label">{k}</div>
            </div>
          ))}
        </div>
      )}

      <div className="tabs">
        {TABS.map((t) => (
          <button key={t} className={`tab ${tab === t ? "tab-active" : ""}`} onClick={() => setTab(t)}>{t}</button>
        ))}
      </div>

      {tab === "Jobs" && <Jobs queueId={queue.id} retryPolicies={retryPolicies} scripts={scripts} />}
      {tab === "Scheduled jobs" && <ScheduledJobs queueId={queue.id} retryPolicies={retryPolicies} scripts={scripts} />}
      {tab === "Dead letter queue" && <Dlq queueId={queue.id} />}
    </div>
  );
}
