import { useState } from "react";
import { buildPayload } from "../jobTypes";
import PayloadPicker from "./PayloadPicker";

const emptyForm = {
  name: "",
  scheduled_type: "immediate",
  delay_seconds: 30,
  scheduled_time: "",
  retries_max: 3,
  retry_policy_id: "",
  typeId: "python",
  fieldValues: {},
  rawPayload: "{}",
};

export default function JobForm({ retryPolicies, scripts, onSubmit, busy }) {
  const [form, setForm] = useState(emptyForm);
  const [error, setError] = useState("");

  async function submit(e) {
    e.preventDefault();
    setError("");

    const { payload, error: buildError } = buildPayload(form.typeId, form.fieldValues, form.rawPayload);
    if (buildError) {
      setError(buildError);
      return;
    }

    const body = {
      name: form.name,
      scheduled_type: form.scheduled_type,
      payload,
      retries_max: Number(form.retries_max) || 3,
      retry_policy_id: form.retry_policy_id || undefined,
    };
    if (form.scheduled_type === "delayed") body.delay_seconds = Number(form.delay_seconds) || 0;
    if (form.scheduled_type === "scheduled") body.scheduled_time = new Date(form.scheduled_time).toISOString();

    const ok = await onSubmit(body);
    if (ok) setForm(emptyForm);
  }

  return (
    <form className="card job-form" onSubmit={submit}>
      <div className="job-form-row">
        <input
          placeholder="Job name"
          value={form.name}
          onChange={(e) => setForm({ ...form, name: e.target.value })}
          required
        />
        <select value={form.scheduled_type} onChange={(e) => setForm({ ...form, scheduled_type: e.target.value })}>
          <option value="immediate">Immediate</option>
          <option value="delayed">Delayed</option>
          <option value="scheduled">Scheduled (at time)</option>
        </select>
        {form.scheduled_type === "delayed" && (
          <input
            type="number"
            min={0}
            placeholder="Delay (seconds)"
            value={form.delay_seconds}
            onChange={(e) => setForm({ ...form, delay_seconds: e.target.value })}
          />
        )}
        {form.scheduled_type === "scheduled" && (
          <input
            type="datetime-local"
            value={form.scheduled_time}
            onChange={(e) => setForm({ ...form, scheduled_time: e.target.value })}
            required
          />
        )}
        <input
          type="number"
          min={0}
          placeholder="Max retries"
          value={form.retries_max}
          onChange={(e) => setForm({ ...form, retries_max: e.target.value })}
        />
        <select value={form.retry_policy_id} onChange={(e) => setForm({ ...form, retry_policy_id: e.target.value })}>
          <option value="">Queue default retry policy</option>
          {retryPolicies.map((p) => (
            <option key={p.id} value={p.id}>{p.name}</option>
          ))}
        </select>
      </div>

      <PayloadPicker
        typeId={form.typeId}
        fieldValues={form.fieldValues}
        rawPayload={form.rawPayload}
        scripts={scripts}
        onTypeChange={(typeId) => setForm((f) => ({ ...f, typeId, fieldValues: {} }))}
        onFieldChange={(key, value) => setForm((f) => ({ ...f, fieldValues: { ...f.fieldValues, [key]: value } }))}
        onRawChange={(rawPayload) => setForm((f) => ({ ...f, rawPayload }))}
      />

      {error && <div className="error">{error}</div>}

      <button className="btn-primary" type="submit" disabled={busy}>Create job</button>
    </form>
  );
}
