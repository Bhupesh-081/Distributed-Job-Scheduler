import { useState } from "react";
import * as api from "../api";
import { buildPayload } from "../jobTypes";
import PayloadPicker from "./PayloadPicker";

let nextRowKey = 1;
function newRow() {
  return { key: nextRowKey++, name: "", typeId: "python", fieldValues: {}, rawPayload: "{}" };
}

const SCHEDULE_TYPES = [
  { id: "immediate", label: "Immediate" },
  { id: "delayed", label: "Delayed" },
  { id: "scheduled", label: "Scheduled (at time)" },
  { id: "recurring", label: "Recurring (cron)" },
];

// One shared schedule (immediate/delayed/scheduled/cron) applied to one or
// more jobs at once - immediate/delayed/scheduled go through the existing
// atomic POST /jobs/batch; recurring has no batch endpoint on the backend,
// so each row becomes its own POST .../scheduled-jobs call (not atomic -
// a later row can fail after earlier ones already exist, unlike the real
// batch endpoint).
export default function JobScheduling({ queue, retryPolicies, scripts }) {
  const [scheduleType, setScheduleType] = useState("immediate");
  const [delaySeconds, setDelaySeconds] = useState(30);
  const [scheduledTime, setScheduledTime] = useState("");
  const [cronExpression, setCronExpression] = useState("*/5 * * * *");
  const [retriesMax, setRetriesMax] = useState(3);
  const [retryPolicyId, setRetryPolicyId] = useState("");
  const [rows, setRows] = useState([newRow()]);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);

  function updateRow(key, patch) {
    setRows((rs) => rs.map((r) => (r.key === key ? { ...r, ...patch } : r)));
  }

  function addRow() {
    setRows((rs) => [...rs, newRow()]);
  }

  function removeRow(key) {
    setRows((rs) => (rs.length > 1 ? rs.filter((r) => r.key !== key) : rs));
  }

  async function submit(e) {
    e.preventDefault();
    setError("");
    setNotice("");

    const built = [];
    for (const row of rows) {
      if (!row.name.trim()) {
        setError("Every job needs a name.");
        return;
      }
      const { payload, error: buildError } = buildPayload(row.typeId, row.fieldValues, row.rawPayload);
      if (buildError) {
        setError(`${row.name}: ${buildError}`);
        return;
      }
      built.push({ name: row.name.trim(), payload });
    }

    setBusy(true);
    try {
      if (scheduleType === "recurring") {
        for (const { name, payload } of built) {
          await api.createScheduledJob(queue.id, {
            name,
            cron_expression: cronExpression,
            payload,
            retries_max: Number(retriesMax) || 3,
            retry_policy_id: retryPolicyId || undefined,
          });
        }
      } else {
        const jobs = built.map(({ name, payload }) => {
          const job = {
            name,
            scheduled_type: scheduleType,
            payload,
            queue_id: queue.id,
            retries_max: Number(retriesMax) || 3,
            retry_policy_id: retryPolicyId || undefined,
          };
          if (scheduleType === "delayed") job.delay_seconds = Number(delaySeconds) || 0;
          if (scheduleType === "scheduled") job.scheduled_time = new Date(scheduledTime).toISOString();
          return job;
        });
        await api.createJobsBatch(jobs);
      }
      setNotice(`${built.length} job${built.length > 1 ? "s" : ""} created.`);
      setRows([newRow()]);
    } catch (err) {
      setError(err.message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <div>
      <form className="card job-form" onSubmit={submit}>
        <div className="job-form-row">
          <select value={scheduleType} onChange={(e) => setScheduleType(e.target.value)}>
            {SCHEDULE_TYPES.map((t) => <option key={t.id} value={t.id}>{t.label}</option>)}
          </select>
          {scheduleType === "delayed" && (
            <input
              type="number"
              min={0}
              placeholder="Delay (seconds)"
              value={delaySeconds}
              onChange={(e) => setDelaySeconds(e.target.value)}
            />
          )}
          {scheduleType === "scheduled" && (
            <input
              type="datetime-local"
              value={scheduledTime}
              onChange={(e) => setScheduledTime(e.target.value)}
              required
            />
          )}
          {scheduleType === "recurring" && (
            <input
              placeholder="Cron expression (*/5 * * * *)"
              value={cronExpression}
              onChange={(e) => setCronExpression(e.target.value)}
              required
            />
          )}
          <input
            type="number"
            min={0}
            placeholder="Max retries"
            value={retriesMax}
            onChange={(e) => setRetriesMax(e.target.value)}
          />
          <select value={retryPolicyId} onChange={(e) => setRetryPolicyId(e.target.value)}>
            <option value="">Queue default retry policy</option>
            {retryPolicies.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
          </select>
        </div>
        {scheduleType === "recurring" && (
          <p className="muted" style={{ margin: 0 }}>
            Every job below is created as its own recurring definition, all sharing this cron expression.
          </p>
        )}

        {rows.map((row, i) => (
          <div className="card job-schedule-row" key={row.key}>
            <div className="job-schedule-row-header">
              <input
                placeholder={`Job ${i + 1} name`}
                value={row.name}
                onChange={(e) => updateRow(row.key, { name: e.target.value })}
                required
              />
              {rows.length > 1 && (
                <button type="button" className="link danger" onClick={() => removeRow(row.key)}>Remove</button>
              )}
            </div>
            <PayloadPicker
              typeId={row.typeId}
              fieldValues={row.fieldValues}
              rawPayload={row.rawPayload}
              scripts={scripts}
              onTypeChange={(typeId) => updateRow(row.key, { typeId, fieldValues: {} })}
              onFieldChange={(key, value) => updateRow(row.key, { fieldValues: { ...row.fieldValues, [key]: value } })}
              onRawChange={(rawPayload) => updateRow(row.key, { rawPayload })}
            />
          </div>
        ))}

        <div className="job-schedule-actions">
          <button type="button" className="link" onClick={addRow}>+ Add another job</button>
        </div>

        {notice && <div className="notice">{notice}</div>}
        {error && <div className="error">{error}</div>}

        <button className="btn-primary" type="submit" disabled={busy}>
          {rows.length > 1 ? `Create ${rows.length} jobs` : "Create job"}
        </button>
      </form>
    </div>
  );
}
