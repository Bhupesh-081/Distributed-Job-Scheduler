import { useState } from "react";
import * as api from "../api";

const emptyForm = { name: "", strategy: "fixed", base_delay_seconds: 5, max_delay_seconds: "" };

export default function RetryPolicies({ projectId, policies, refresh }) {
  const [form, setForm] = useState(emptyForm);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  async function create(e) {
    e.preventDefault();
    setError("");
    setBusy(true);
    try {
      await api.createRetryPolicy(projectId, {
        name: form.name,
        strategy: form.strategy,
        base_delay_seconds: Number(form.base_delay_seconds),
        max_delay_seconds: form.max_delay_seconds ? Number(form.max_delay_seconds) : undefined,
      });
      setForm(emptyForm);
      refresh();
    } catch (err) {
      setError(err.message);
    } finally {
      setBusy(false);
    }
  }

  async function remove(id) {
    setError("");
    try {
      await api.deleteRetryPolicy(id);
      refresh();
    } catch (err) {
      setError(err.message);
    }
  }

  return (
    <div>
      <form className="card form-grid" onSubmit={create}>
        <input placeholder="Name" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} required />
        <select value={form.strategy} onChange={(e) => setForm({ ...form, strategy: e.target.value })}>
          <option value="fixed">Fixed</option>
          <option value="linear">Linear backoff</option>
          <option value="exponential">Exponential backoff</option>
        </select>
        <input
          type="number"
          min={1}
          placeholder="Base delay (seconds)"
          value={form.base_delay_seconds}
          onChange={(e) => setForm({ ...form, base_delay_seconds: e.target.value })}
          required
        />
        <input
          type="number"
          min={1}
          placeholder="Max delay (optional)"
          value={form.max_delay_seconds}
          onChange={(e) => setForm({ ...form, max_delay_seconds: e.target.value })}
        />
        <button className="btn-primary" type="submit" disabled={busy}>Create retry policy</button>
      </form>

      {error && <div className="error">{error}</div>}

      <table className="table">
        <thead>
          <tr><th>Name</th><th>Strategy</th><th>Base delay</th><th>Max delay</th><th></th></tr>
        </thead>
        <tbody>
          {policies.map((p) => (
            <tr key={p.id}>
              <td>{p.name}</td>
              <td>{p.strategy}</td>
              <td>{p.base_delay_seconds}s</td>
              <td>{p.max_delay_seconds ?? "-"}</td>
              <td><button className="link danger" onClick={() => remove(p.id)}>Delete</button></td>
            </tr>
          ))}
          {policies.length === 0 && (
            <tr><td colSpan={5} className="muted">No retry policies yet - new jobs fall back to a fixed 5s delay.</td></tr>
          )}
        </tbody>
      </table>
    </div>
  );
}
