import { useState } from "react";
import * as api from "../api";

const emptyForm = { name: "", script_type: "python", content: "" };

export default function Scripts({ projectId, scripts, refresh }) {
  const [form, setForm] = useState(emptyForm);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  async function create(e) {
    e.preventDefault();
    setError("");
    setBusy(true);
    try {
      await api.createScript(projectId, form);
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
      await api.deleteScript(id);
      refresh();
    } catch (err) {
      setError(err.message);
    }
  }

  return (
    <div>
      <p className="muted" style={{ marginBottom: 14 }}>
        Save a Python or Bash script here once, then pick it from the "Load from library" menu when creating a job
        instead of retyping it into the code editor every time.
      </p>

      <form className="card job-form" onSubmit={create}>
        <div className="job-form-row">
          <input placeholder="Script name" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} required />
          <select value={form.script_type} onChange={(e) => setForm({ ...form, script_type: e.target.value })}>
            <option value="python">Python</option>
            <option value="bash">Bash</option>
          </select>
        </div>
        <textarea
          className="code-input"
          placeholder={form.script_type === "python" ? 'print("hello from the library")' : 'echo "hello from the library"'}
          value={form.content}
          onChange={(e) => setForm({ ...form, content: e.target.value })}
          spellCheck={false}
          required
        />
        <button className="btn-primary" type="submit" disabled={busy}>Save script</button>
      </form>

      {error && <div className="error">{error}</div>}

      <table className="table">
        <thead>
          <tr><th>Name</th><th>Type</th><th>Updated</th><th></th></tr>
        </thead>
        <tbody>
          {scripts.map((s) => (
            <tr key={s.id}>
              <td>{s.name}</td>
              <td>{s.script_type}</td>
              <td>{new Date(s.updated_at).toLocaleString()}</td>
              <td><button className="link danger" onClick={() => remove(s.id)}>Delete</button></td>
            </tr>
          ))}
          {scripts.length === 0 && (
            <tr><td colSpan={4} className="muted">No saved scripts yet.</td></tr>
          )}
        </tbody>
      </table>
    </div>
  );
}
