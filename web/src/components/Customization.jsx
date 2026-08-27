import { useEffect, useState } from "react";
import * as api from "../api";
import { getSettings } from "../settings";
import { useQueuePicker } from "../useQueuePicker";
import QueuePickerBar from "./QueuePickerBar";
import QueueConfig from "./QueueConfig";

// A flat, top-level home for queue customization (priority, concurrency,
// retry policy, pause/resume, live stats) - instead of being buried in the
// Organizations drill-down. Job creation/scheduling lives in its own
// "Job Scheduler" section (see JobSchedulerPage.jsx); this page is queue
// config only.
export default function Customization() {
  const picker = useQueuePicker();
  const { queueId, setQueues, queue, retryPolicies, error, setError } = picker;

  const [stats, setStats] = useState(null);

  useEffect(() => {
    if (!queueId) {
      setStats(null);
      return;
    }
    let stopped = false;
    async function poll() {
      try {
        const s = await api.getQueueStats(queueId);
        if (!stopped) setStats(s);
      } catch (err) {
        if (!stopped) setError(err.message);
      }
    }
    poll();
    const t = setInterval(poll, getSettings().pollMs);
    return () => {
      stopped = true;
      clearInterval(t);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [queueId]);

  function applyUpdatedQueue(updated) {
    setQueues((qs) => qs.map((q) => (q.id === updated.id ? updated : q)));
  }

  async function toggle() {
    setError("");
    try {
      applyUpdatedQueue(queue.paused ? await api.resumeQueue(queue.id) : await api.pauseQueue(queue.id));
    } catch (err) {
      setError(err.message);
    }
  }

  return (
    <div>
      <div className="content-header"><h2>Customization</h2></div>
      <p className="muted" style={{ marginBottom: 16 }}>
        Pick a queue to configure its priority, concurrency, and default retry policy, pause or resume it, and see
        its live stats - all in one place.
      </p>

      <QueuePickerBar picker={picker} />

      {error && <div className="error">{error}</div>}

      {queue ? (
        <>
          <div className="content-header" style={{ marginTop: 24 }}>
            <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
              <h3 style={{ margin: 0 }}>{queue.name}</h3>
              <span className={`pill ${queue.paused ? "pill-warn" : "pill-good"}`}>
                {queue.paused ? "Paused" : "Active"}
              </span>
            </div>
            <button className="btn-primary" onClick={toggle}>
              {queue.paused ? "Resume queue" : "Pause queue"}
            </button>
          </div>

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

          <QueueConfig queue={queue} retryPolicies={retryPolicies} onChanged={applyUpdatedQueue} />
        </>
      ) : (
        <p className="muted" style={{ marginTop: 24 }}>Select a queue above to configure it.</p>
      )}
    </div>
  );
}
