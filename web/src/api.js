const BASE = import.meta.env.VITE_API_URL || "http://localhost:8080";

let accessToken = null;

function setSession(tokens) {
  accessToken = tokens?.access_token ?? null;
  if (tokens?.refresh_token) {
    localStorage.setItem("refresh_token", tokens.refresh_token);
  } else {
    localStorage.removeItem("refresh_token");
  }
}

async function raw(path, opts = {}) {
  const res = await fetch(BASE + path, {
    method: opts.body ? "POST" : "GET",
    ...opts,
    headers: {
      ...(opts.body ? { "Content-Type": "application/json" } : {}),
      ...(accessToken ? { Authorization: `Bearer ${accessToken}` } : {}),
      ...opts.headers,
    },
    body: opts.body ? JSON.stringify(opts.body) : undefined,
  });
  const text = await res.text();
  const data = text ? JSON.parse(text) : null;
  if (!res.ok) throw new Error(data?.error?.message || `request failed (${res.status})`);
  return data;
}

// One retry-after-refresh for protected calls, since the 15m access token
// can expire mid-session.
async function authed(path, opts = {}) {
  try {
    return await raw(path, opts);
  } catch (err) {
    const rt = localStorage.getItem("refresh_token");
    if (!rt) throw err;
    const tokens = await raw("/auth/refresh", { body: { refresh_token: rt } });
    setSession(tokens);
    return raw(path, opts);
  }
}

export async function register(email, password) {
  const tokens = await raw("/auth/register", { body: { email, password } });
  setSession(tokens);
}

export async function login(email, password) {
  const tokens = await raw("/auth/login", { body: { email, password } });
  setSession(tokens);
}

export async function logout() {
  const rt = localStorage.getItem("refresh_token");
  if (rt) {
    try {
      await raw("/auth/logout", { body: { refresh_token: rt } });
    } catch {
      // token may already be invalid — fall through to local cleanup
    }
  }
  setSession(null);
}

export function isLoggedIn() {
  return !!accessToken || !!localStorage.getItem("refresh_token");
}

export const listOrganizations = () => authed("/organizations");
export const createOrganization = (name) => authed("/organizations", { body: { name } });
