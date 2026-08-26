import { useEffect, useState } from "react";
import * as api from "../api";
import { fmtDate } from "../format";
import { buildPayload } from "../jobTypes";
import PayloadPicker from "./PayloadPicker";

const emptyForm = {
  name: "",
  cron_expression: "*/5 * * * *",
  retries_max: 3,
  retry_policy_id: "",
  typeId: "python",
  fieldValues: {},
  rawPayload: "{}",
};

export default function ScheduledJobs({ queueId, retryPolicies }) {
  const [items, setItems] = useState([]);
  const [form, setForm] = useState(emptyForm);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  async function refresh() {
    setError("");
    try {
      setItems(await api.listScheduledJobs(queueId));
    } catch (err) {
      setError(err.message);
    }
  }

  useEffect(() => {
    refresh();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [queueId]);

  async function create(e) {
    e.preventDefault();
    setError("");
    const { payload, error: buildError } = buildPayload(form.typeId, form.fieldValues, form.rawPayload);
    if (buildError) {
      setError(buildError);
      return;
    }
    setBusy(true);
    try {
      await api.createScheduledJob(queueId, {
        name: form.name,
        cron_expression: form.cron_expression,
        payload,
        retries_max: Number(form.retries_max) || 3,
        retry_policy_id: form.retry_policy_id || undefined,
      });
      setForm(emptyForm);
      refresh();
    } catch (err) {
      setError(err.message);
    } finally {
      setBusy(false);
    }
  }

  async function toggle(sj) {
    setError("");
    try {
      sj.active ? await api.pauseScheduledJob(sj.id) : await api.resumeScheduledJob(sj.id);
      refresh();
    } catch (err) {
      setError(err.message);
    }
  }

  async function remove(id) {
    setError("");
    try {
      await api.deleteScheduledJob(id);
      refresh();
    } catch (err) {
      setError(err.message);
    }
  }

  return (
    <div>
      <form className="card job-form" onSubmit={create}>
        <div className="job-form-row">
          <input placeholder="Name" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} required />
          <input
            placeholder="Cron expression (*/5 * * * *)"
            value={form.cron_expression}
            onChange={(e) => setForm({ ...form, cron_expression: e.target.value })}
            required
          />
          <input
            type="number"
            min={0}
            placeholder="Max retries"
            value={form.retries_max}
            onChange={(e) => setForm({ ...form, retries_max: e.target.value })}
          />
          <select value={form.retry_policy_id} onChange={(e) => setForm({ ...form, retry_policy_id: e.target.value })}>
            <option value="">Queue default retry policy</option>
            {retryPolicies.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
          </select>
        </div>

        <PayloadPicker
          typeId={form.typeId}
          fieldValues={form.fieldValues}
          rawPayload={form.rawPayload}
          onTypeChange={(typeId) => setForm((f) => ({ ...f, typeId, fieldValues: {} }))}
          onFieldChange={(key, value) => setForm((f) => ({ ...f, fieldValues: { ...f.fieldValues, [key]: value } }))}
          onRawChange={(rawPayload) => setForm((f) => ({ ...f, rawPayload }))}
        />

        <button className="btn-primary" type="submit" disabled={busy}>Create scheduled job</button>
      </form>

      {error && <div className="error">{error}</div>}

      <table className="table">
        <thead>
          <tr>
            <th>Name</th><th>Cron</th><th>Active</th><th>Next run</th><th>Last run</th><th></th>
          </tr>
        </thead>
        <tbody>
          {items.map((sj) => (
            <tr key={sj.id}>
              <td>{sj.name}</td>
              <td className="mono">{sj.cron_expression}</td>
              <td>{sj.active ? "yes" : "paused"}</td>
              <td>{fmtDate(sj.next_run_at)}</td>
              <td>{fmtDate(sj.last_run_at)}</td>
              <td className="row-actions">
                <button className="link" onClick={() => toggle(sj)}>{sj.active ? "Pause" : "Resume"}</button>
                <button className="link danger" onClick={() => remove(sj.id)}>Delete</button>
              </td>
            </tr>
          ))}
          {items.length === 0 && (
            <tr><td colSpan={6} className="muted">No scheduled jobs for this queue.</td></tr>
          )}
        </tbody>
      </table>
    </div>
  );
}
