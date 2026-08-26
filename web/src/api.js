const API_BASE = import.meta.env.VITE_API_URL || "http://localhost:8080";
const JOB_BASE = import.meta.env.VITE_JOB_API_URL || "http://localhost:8081";

let accessToken = null;

function setSession(tokens) {
  accessToken = tokens?.access_token ?? null;
  if (tokens?.refresh_token) {
    localStorage.setItem("refresh_token", tokens.refresh_token);
  } else {
    localStorage.removeItem("refresh_token");
  }
}

async function raw(base, path, opts = {}) {
  const res = await fetch(base + path, {
    method: opts.method || (opts.body ? "POST" : "GET"),
    ...opts,
    headers: {
      ...(opts.body ? { "Content-Type": "application/json" } : {}),
      ...(accessToken ? { Authorization: `Bearer ${accessToken}` } : {}),
      ...opts.headers,
    },
    body: opts.body ? JSON.stringify(opts.body) : undefined,
  });
  if (res.status === 204) return null;
  const text = await res.text();
  const data = text ? JSON.parse(text) : null;
  if (!res.ok) throw new Error(data?.error?.message || `request failed (${res.status})`);
  return data;
}

// One retry-after-refresh for protected calls, since the 15m access token
// can expire mid-session.
async function authed(base, path, opts = {}) {
  try {
    return await raw(base, path, opts);
  } catch (err) {
    const rt = localStorage.getItem("refresh_token");
    if (!rt) throw err;
    const tokens = await raw(API_BASE, "/auth/refresh", { body: { refresh_token: rt } });
    setSession(tokens);
    return raw(base, path, opts);
  }
}

const api = (path, opts) => authed(API_BASE, path, opts);
const jobApi = (path, opts) => authed(JOB_BASE, path, opts);

function qs(params = {}) {
  const parts = Object.entries(params)
    .filter(([, v]) => v !== undefined && v !== null && v !== "")
    .map(([k, v]) => `${encodeURIComponent(k)}=${encodeURIComponent(v)}`);
  return parts.length ? `?${parts.join("&")}` : "";
}

export async function register(email, password) {
  const tokens = await raw(API_BASE, "/auth/register", { body: { email, password } });
  setSession(tokens);
}

export async function login(email, password) {
  const tokens = await raw(API_BASE, "/auth/login", { body: { email, password } });
  setSession(tokens);
}

export async function logout() {
  const rt = localStorage.getItem("refresh_token");
  if (rt) {
    try {
      await raw(API_BASE, "/auth/logout", { body: { refresh_token: rt } });
    } catch {
      // token may already be invalid - fall through to local cleanup
    }
  }
  setSession(null);
}

export function isLoggedIn() {
  return !!accessToken || !!localStorage.getItem("refresh_token");
}

// --- organizations / projects ---
export const listOrganizations = () => api("/organizations");
export const createOrganization = (name) => api("/organizations", { body: { name } });
export const listProjects = (orgId) => api(`/organizations/${orgId}/projects`);
export const createProject = (orgId, name) => api(`/organizations/${orgId}/projects`, { body: { name } });

// --- queues ---
export const listQueues = (projectId) => api(`/projects/${projectId}/queues`);
export const createQueue = (projectId, body) => api(`/projects/${projectId}/queues`, { body });
export const updateQueue = (queueId, body) => api(`/queues/${queueId}`, { method: "PATCH", body });
export const deleteQueue = (queueId) => api(`/queues/${queueId}`, { method: "DELETE" });
export const pauseQueue = (queueId) => api(`/queues/${queueId}/pause`, { method: "POST", body: {} });
export const resumeQueue = (queueId) => api(`/queues/${queueId}/resume`, { method: "POST", body: {} });
export const getQueueStats = (queueId) => api(`/queues/${queueId}/stats`);

// --- retry policies ---
export const listRetryPolicies = (projectId) => api(`/projects/${projectId}/retry-policies`);
export const createRetryPolicy = (projectId, body) => api(`/projects/${projectId}/retry-policies`, { body });
export const updateRetryPolicy = (id, body) => api(`/retry-policies/${id}`, { method: "PATCH", body });
export const deleteRetryPolicy = (id) => api(`/retry-policies/${id}`, { method: "DELETE" });

// Live job updates for one queue. Browsers' WebSocket API can't set an
// Authorization header, so the token travels as a query param - the
// backend route (GET /jobs/stream) accepts that specifically for this.
export function openJobsStream(queueId, { onJob, onOpen, onClose } = {}) {
  const wsBase = JOB_BASE.replace(/^http/, "ws");
  const url = `${wsBase}/jobs/stream?queue_id=${encodeURIComponent(queueId)}&token=${encodeURIComponent(accessToken || "")}`;
  const ws = new WebSocket(url);
  ws.onopen = () => onOpen?.();
  ws.onclose = () => onClose?.();
  ws.onmessage = (event) => {
    try {
      const msg = JSON.parse(event.data);
      if (msg.type === "job_updated") onJob?.(msg.job);
    } catch {
      // ignore malformed frames
    }
  };
  return ws;
}

// --- jobs (job-service, separate port) ---
export const listJobs = (queueId, { status, page, page_size } = {}) =>
  jobApi(`/jobs${qs({ queue_id: queueId, status, page, page_size })}`);
export const getJob = (id) => jobApi(`/jobs/${id}`);
export const createJob = (body) => jobApi("/jobs", { body });
export const getJobLogs = (id) => jobApi(`/jobs/${id}/logs`);
export const cancelJob = (id) => jobApi(`/jobs/${id}/cancel`, { method: "POST", body: {} });

// --- workers ---
export const listWorkers = (status) => api(`/workers${qs({ status })}`);
export const getWorker = (id) => api(`/workers/${id}`);

// --- dead letter queue ---
export const listDLQ = (queueId) => api(`/queues/${queueId}/dlq`);
export const replayDLQEntry = (id) => api(`/dlq/${id}/replay`, { method: "POST", body: {} });
export const deleteDLQEntry = (id) => api(`/dlq/${id}`, { method: "DELETE" });

// --- scheduled (cron) jobs ---
export const listScheduledJobs = (queueId) => api(`/queues/${queueId}/scheduled-jobs`);
export const createScheduledJob = (queueId, body) => api(`/queues/${queueId}/scheduled-jobs`, { body });
export const updateScheduledJob = (id, body) => api(`/scheduled-jobs/${id}`, { method: "PATCH", body });
export const deleteScheduledJob = (id) => api(`/scheduled-jobs/${id}`, { method: "DELETE" });
export const pauseScheduledJob = (id) => api(`/scheduled-jobs/${id}/pause`, { method: "POST", body: {} });
export const resumeScheduledJob = (id) => api(`/scheduled-jobs/${id}/resume`, { method: "POST", body: {} });

// --- system ---
export const getMetrics = () => api("/system/metrics");
