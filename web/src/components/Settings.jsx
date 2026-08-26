import { useState } from "react";
import { getSettings, saveSettings, applyTheme } from "../settings";
import { loadCustomTypes, saveCustomTypes } from "../jobTypes";
import { IconWrench } from "./Icons";

const emptyNewType = { label: "", cmd: "", argsHint: "" };

export default function Settings() {
  const [settings, setSettings] = useState(getSettings());
  const [customTypes, setCustomTypes] = useState(loadCustomTypes());
  const [newType, setNewType] = useState(emptyNewType);

  function updateTheme(theme) {
    setSettings(saveSettings({ theme }));
    applyTheme(theme);
  }

  function updatePoll(pollMs) {
    setSettings(saveSettings({ pollMs: Number(pollMs) }));
  }

  function addType(e) {
    e.preventDefault();
    if (!newType.label.trim() || !newType.cmd.trim()) return;
    const next = [...customTypes, { id: `custom-${Date.now()}`, ...newType }];
    saveCustomTypes(next);
    setCustomTypes(next);
    setNewType(emptyNewType);
  }

  function removeType(id) {
    const next = customTypes.filter((t) => t.id !== id);
    saveCustomTypes(next);
    setCustomTypes(next);
  }

  return (
    <div>
      <div className="content-header"><h2>Settings</h2></div>

      <div className="card settings-card">
        <h3>Appearance</h3>
        <div className="settings-row">
          <span>Theme</span>
          <div className="segmented">
            {["dark", "light"].map((t) => (
              <button
                key={t}
                type="button"
                className={`segmented-btn ${settings.theme === t ? "segmented-active" : ""}`}
                onClick={() => updateTheme(t)}
              >
                {t}
              </button>
            ))}
          </div>
        </div>
        <div className="settings-row">
          <span>Live refresh interval</span>
          <select value={settings.pollMs} onChange={(e) => updatePoll(e.target.value)}>
            <option value={3000}>3s</option>
            <option value={6000}>6s</option>
            <option value={10000}>10s</option>
            <option value={30000}>30s</option>
          </select>
        </div>
        <p className="muted settings-hint">Applies the next time a page starts polling - switch tabs to pick it up.</p>
      </div>

      <div className="card settings-card">
        <h3>Job types</h3>
        <p className="muted">
          Built in: shell command, Python script, Node.js script, Docker container, HTTP request, custom JSON.
          Add your own below - they show up in the job creation form.
        </p>
        <form className="settings-jobtype-form" onSubmit={addType}>
          <input placeholder="Label (e.g. Go binary)" value={newType.label} onChange={(e) => setNewType({ ...newType, label: e.target.value })} />
          <input placeholder="Base command (e.g. ./my-binary)" value={newType.cmd} onChange={(e) => setNewType({ ...newType, cmd: e.target.value })} />
          <input placeholder="Arguments field label" value={newType.argsHint} onChange={(e) => setNewType({ ...newType, argsHint: e.target.value })} />
          <button className="btn-primary" type="submit">Add job type</button>
        </form>
        <ul className="jobtype-list">
          {customTypes.map((t) => (
            <li key={t.id} className="jobtype-item">
              <span className="jobtype-item-label"><IconWrench size={14} /> {t.label}</span>
              <span className="mono muted">{t.cmd}</span>
              <button type="button" className="link danger" onClick={() => removeType(t.id)}>Remove</button>
            </li>
          ))}
          {customTypes.length === 0 && <li className="muted">No custom job types yet.</li>}
        </ul>
      </div>

      <div className="card settings-card">
        <h3>Connection</h3>
        <div className="settings-row"><span>API</span><span className="mono">{import.meta.env.VITE_API_URL || "http://localhost:8080"}</span></div>
        <div className="settings-row"><span>Job service</span><span className="mono">{import.meta.env.VITE_JOB_API_URL || "http://localhost:8081"}</span></div>
      </div>
    </div>
  );
}
