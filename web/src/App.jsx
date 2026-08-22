import { useEffect, useState } from "react";
import * as api from "./api";

function AuthForm({ onAuthed }) {
  const [mode, setMode] = useState("login"); // "login" | "register"
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  async function submit(e) {
    e.preventDefault();
    setError("");
    setBusy(true);
    try {
      if (mode === "login") await api.login(email, password);
      else await api.register(email, password);
      onAuthed();
    } catch (err) {
      setError(err.message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="auth-screen">
      <form className="card auth-card" onSubmit={submit}>
        <h1 className="brand">Job Scheduler</h1>
        <p className="subtitle">{mode === "login" ? "Sign in to your account" : "Create an account"}</p>

        <label>
          Email
          <input type="email" value={email} onChange={(e) => setEmail(e.target.value)} required autoFocus />
        </label>
        <label>
          Password
          <input
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            minLength={8}
            required
          />
        </label>

        {error && <div className="error">{error}</div>}

        <button className="btn-primary" type="submit" disabled={busy}>
          {busy ? "…" : mode === "login" ? "Sign in" : "Create account"}
        </button>

        <button
          type="button"
          className="link"
          onClick={() => {
            setError("");
            setMode(mode === "login" ? "register" : "login");
          }}
        >
          {mode === "login" ? "Need an account? Register" : "Have an account? Sign in"}
        </button>
      </form>
    </div>
  );
}

function Dashboard({ onLogout }) {
  const [orgs, setOrgs] = useState([]);
  const [name, setName] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);

  async function refresh() {
    setError("");
    try {
      setOrgs(await api.listOrganizations());
    } catch (err) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    refresh();
  }, []);

  async function createOrg(e) {
    e.preventDefault();
    if (!name.trim()) return;
    setError("");
    try {
      await api.createOrganization(name.trim());
      setName("");
      refresh();
    } catch (err) {
      setError(err.message);
    }
  }

  return (
    <div className="shell">
      <header className="topbar">
        <span className="brand">Job Scheduler</span>
        <button className="link" onClick={onLogout}>
          Sign out
        </button>
      </header>

      <main className="content">
        <div className="content-header">
          <h2>Organizations</h2>
        </div>

        <form className="card inline-form" onSubmit={createOrg}>
          <input
            placeholder="New organization name"
            value={name}
            onChange={(e) => setName(e.target.value)}
          />
          <button className="btn-primary" type="submit">
            Create
          </button>
        </form>

        {error && <div className="error">{error}</div>}

        {loading ? (
          <p className="muted">Loading…</p>
        ) : orgs.length === 0 ? (
          <p className="muted">No organizations yet — create one above.</p>
        ) : (
          <ul className="org-list">
            {orgs.map((o) => (
              <li key={o.id} className="card org-row">
                <span className="org-name">{o.name}</span>
                <span className="muted">{new Date(o.created_at).toLocaleDateString()}</span>
              </li>
            ))}
          </ul>
        )}
      </main>
    </div>
  );
}

export default function App() {
  const [loggedIn, setLoggedIn] = useState(api.isLoggedIn());

  function handleLogout() {
    api.logout().finally(() => setLoggedIn(false));
  }

  return loggedIn ? (
    <Dashboard onLogout={handleLogout} />
  ) : (
    <AuthForm onAuthed={() => setLoggedIn(true)} />
  );
}
