import { useState } from "react";
import * as api from "../api";

const emptyForm = { name: "", priority: 0, concurrency_limit: 5, default_retry_policy_id: "" };

export default function Queues({ projectId, queues, retryPolicies, refresh, onOpen }) {
  const [form, setForm] = useState(emptyForm);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [nameFilter, setNameFilter] = useState("");

  async function create(e) {
    e.preventDefault();
    setError("");
    setBusy(true);
    try {
      await api.createQueue(projectId, {
        name: form.name,
        priority: Number(form.priority) || 0,
        concurrency_limit: Number(form.concurrency_limit) || 5,
      });
      setForm(emptyForm);
      refresh();
    } catch (err) {
      setError(err.message);
    } finally {
      setBusy(false);
    }
  }

  async function toggle(q) {
    setError("");
    try {
      q.paused ? await api.resumeQueue(q.id) : await api.pauseQueue(q.id);
      refresh();
    } catch (err) {
      setError(err.message);
    }
  }

  async function remove(id) {
    setError("");
    try {
      await api.deleteQueue(id);
      refresh();
    } catch (err) {
      setError(err.message);
    }
  }

  const visibleQueues = nameFilter.trim()
    ? queues.filter((q) => q.name.toLowerCase().includes(nameFilter.trim().toLowerCase()))
    : queues;

  return (
    <div>
      <form className="card form-grid" onSubmit={create}>
        <input placeholder="Queue name" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} required />
        <input
          type="number"
          placeholder="Priority"
          value={form.priority}
          onChange={(e) => setForm({ ...form, priority: e.target.value })}
        />
        <input
          type="number"
          min={1}
          placeholder="Concurrency limit"
          value={form.concurrency_limit}
          onChange={(e) => setForm({ ...form, concurrency_limit: e.target.value })}
        />
        <button className="btn-primary" type="submit" disabled={busy}>Create queue</button>
      </form>

      {error && <div className="error">{error}</div>}

      <div className="content-header">
        <h3 style={{ margin: 0 }}>Queues</h3>
        <input
          placeholder="Filter by name…"
          value={nameFilter}
          onChange={(e) => setNameFilter(e.target.value)}
          style={{ width: 200 }}
        />
      </div>

      <table className="table">
        <thead>
          <tr><th>Name</th><th>Priority</th><th>Concurrency</th><th>Status</th><th></th></tr>
        </thead>
        <tbody>
          {visibleQueues.map((q) => (
            <tr key={q.id}>
              <td>{q.name}</td>
              <td>{q.priority}</td>
              <td>{q.concurrency_limit}</td>
              <td>{q.paused ? "paused" : "active"}</td>
              <td className="row-actions">
                <button className="link" onClick={() => onOpen(q)}>Open</button>
                <button className="link" onClick={() => toggle(q)}>{q.paused ? "Resume" : "Pause"}</button>
                <button className="link danger" onClick={() => remove(q.id)}>Delete</button>
              </td>
            </tr>
          ))}
          {visibleQueues.length === 0 && (
            <tr><td colSpan={5} className="muted">{queues.length === 0 ? "No queues yet - create one above." : "No queues match that filter."}</td></tr>
          )}
        </tbody>
      </table>
      {retryPolicies.length === 0 && (
        <p className="muted" style={{ marginTop: 12 }}>
          Tip: add a retry policy on the Retry Policies tab to assign it to jobs in this project.
        </p>
      )}
    </div>
  );
}
