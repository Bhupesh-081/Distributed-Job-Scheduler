import { useState } from "react";
import * as api from "../api";

// Org -> project -> queue picker + inline "create one if it doesn't exist
// yet" forms - shared by every top-level page that needs "pick a queue"
// without the full Organizations drill-down (Customization, JobScheduler).
export default function QueuePickerBar({ picker }) {
  const { orgs, orgId, setOrgId, projects, projectId, setProjectId, queues, queueId, setQueueId, setError } = picker;
  const [newProjectName, setNewProjectName] = useState("");
  const [creatingProject, setCreatingProject] = useState(false);
  const [newQueueName, setNewQueueName] = useState("");
  const [creatingQueue, setCreatingQueue] = useState(false);

  async function createProject(e) {
    e.preventDefault();
    if (!newProjectName.trim()) return;
    setError("");
    setCreatingProject(true);
    try {
      const created = await api.createProject(orgId, newProjectName.trim());
      setNewProjectName("");
      await picker.refreshProjects();
      setProjectId(created.id);
    } catch (err) {
      setError(err.message);
    } finally {
      setCreatingProject(false);
    }
  }

  async function createQueue(e) {
    e.preventDefault();
    if (!newQueueName.trim()) return;
    setError("");
    setCreatingQueue(true);
    try {
      const created = await api.createQueue(projectId, { name: newQueueName.trim() });
      setNewQueueName("");
      await picker.refreshQueues();
      setQueueId(created.id);
    } catch (err) {
      setError(err.message);
    } finally {
      setCreatingQueue(false);
    }
  }

  return (
    <>
      <div className="card customization-picker">
        <label>
          Organization
          <select value={orgId} onChange={(e) => setOrgId(e.target.value)}>
            <option value="">Select organization…</option>
            {orgs.map((o) => <option key={o.id} value={o.id}>{o.name}</option>)}
          </select>
        </label>
        <label>
          Project
          <select value={projectId} onChange={(e) => setProjectId(e.target.value)} disabled={!orgId}>
            <option value="">Select project…</option>
            {projects.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
          </select>
        </label>
        <label>
          Queue
          <select value={queueId} onChange={(e) => setQueueId(e.target.value)} disabled={!projectId}>
            <option value="">Select queue…</option>
            {queues.map((q) => <option key={q.id} value={q.id}>{q.name}</option>)}
          </select>
        </label>
      </div>

      <form className="card inline-form" style={{ marginTop: 12 }} onSubmit={createProject}>
        <input
          placeholder={orgId ? "New project name" : "Select an organization first"}
          value={newProjectName}
          onChange={(e) => setNewProjectName(e.target.value)}
          disabled={!orgId}
        />
        <button className="btn-primary" type="submit" disabled={!orgId || creatingProject}>Create project</button>
      </form>

      <form className="card inline-form" style={{ marginTop: 12 }} onSubmit={createQueue}>
        <input
          placeholder={projectId ? "New queue name" : "Select a project first"}
          value={newQueueName}
          onChange={(e) => setNewQueueName(e.target.value)}
          disabled={!projectId}
        />
        <button className="btn-primary" type="submit" disabled={!projectId || creatingQueue}>Create queue</button>
      </form>
    </>
  );
}
