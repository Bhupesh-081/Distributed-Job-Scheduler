import { allTypes } from "../jobTypes";
import { TypeIcon } from "./Icons";

// Job-type chips + the type's fields (or raw JSON for "Custom"). Shared by
// JobForm and ScheduledJobs so the python-code editor etc. only exist once.
export default function PayloadPicker({ typeId, fieldValues, rawPayload, scripts, onTypeChange, onFieldChange, onRawChange }) {
  const types = allTypes();
  const type = types.find((t) => t.id === typeId) || types[0];
  const libraryScripts = (scripts || []).filter((s) => s.script_type === type.scriptType);

  return (
    <>
      <div className="job-type-picker">
        {types.map((t) => (
          <button
            type="button"
            key={t.id}
            className={`job-type-chip ${typeId === t.id ? "job-type-chip-active" : ""}`}
            onClick={() => onTypeChange(t.id)}
          >
            <span className="job-type-chip-icon"><TypeIcon iconKey={t.iconKey} size={14} /></span>{t.label}
          </button>
        ))}
      </div>

      {type.id === "raw" ? (
        <textarea
          className="payload-input"
          placeholder="Payload (JSON)"
          value={rawPayload}
          onChange={(e) => onRawChange(e.target.value)}
        />
      ) : (
        <div className="job-type-fields">
          {type.scriptType && libraryScripts.length > 0 && (
            <select
              className="script-library-picker"
              defaultValue=""
              onChange={(e) => {
                const script = libraryScripts.find((s) => s.id === e.target.value);
                if (script) onFieldChange("code", script.content);
                e.target.value = "";
              }}
            >
              <option value="" disabled>Load from library…</option>
              {libraryScripts.map((s) => (
                <option key={s.id} value={s.id}>{s.name}</option>
              ))}
            </select>
          )}
          {type.fields.map((f) =>
            f.type === "code" ? (
              <textarea
                key={f.key}
                className="code-input"
                placeholder={f.placeholder || f.label}
                value={fieldValues[f.key] || ""}
                onChange={(e) => onFieldChange(f.key, e.target.value)}
                spellCheck={false}
              />
            ) : (
              <input
                key={f.key}
                placeholder={f.label + (f.required ? " *" : "")}
                value={fieldValues[f.key] || ""}
                onChange={(e) => onFieldChange(f.key, e.target.value)}
              />
            )
          )}
        </div>
      )}
    </>
  );
}
