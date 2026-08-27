import { useState } from "react";
import * as api from "../api";

// Pause/resume and live statistics already live in QueueDetail's header,
// above these tabs - this tab is only the config that actually needs a
// form: priority, concurrency, and which retry policy applies by default.
export default function QueueConfig({ queue, retryPolicies, onChanged }) {
  const [priority, setPriority] = useState(queue.priority);
  const [concurrencyLimit, setConcurrencyLimit] = useState(queue.concurrency_limit);
  const [retryPolicyId, setRetryPolicyId] = useState(queue.default_retry_policy_id || "");
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);

  async function save(e) {
    e.preventDefault();
    setError("");
    setNotice("");
    setBusy(true);
    try {
      const updated = await api.updateQueue(queue.id, {
        name: queue.name,
        priority: Number(priority) || 0,
        concurrency_limit: Number(concurrencyLimit) || 1,
        default_retry_policy_id: retryPolicyId || undefined,
      });
      onChanged(updated);
      setNotice("Queue configuration saved.");
    } catch (err) {
      setError(err.message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="card settings-card">
      <h3>Queue configuration</h3>
      <p className="muted">
        Priority and concurrency control how watcher-service dispatches jobs from this queue against every other
        queue's; the default retry policy applies to any job here that doesn't set its own override.
      </p>
      <form className="stacked-form" onSubmit={save}>
        <label>
          Priority
          <input type="number" value={priority} onChange={(e) => setPriority(e.target.value)} required />
        </label>
        <label>
          Concurrency limit
          <input
            type="number"
            min={1}
            value={concurrencyLimit}
            onChange={(e) => setConcurrencyLimit(e.target.value)}
            required
          />
        </label>
        <label>
          Default retry policy
          <select value={retryPolicyId} onChange={(e) => setRetryPolicyId(e.target.value)}>
            <option value="">None (fixed 5s fallback)</option>
            {retryPolicies.map((p) => (
              <option key={p.id} value={p.id}>{p.name}</option>
            ))}
          </select>
        </label>

        {notice && <div className="notice">{notice}</div>}
        {error && <div className="error">{error}</div>}

        <button className="btn-primary" type="submit" disabled={busy}>Save configuration</button>
      </form>
    </div>
  );
}
