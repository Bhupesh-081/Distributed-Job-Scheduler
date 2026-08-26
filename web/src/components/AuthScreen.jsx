import { useState } from "react";
import * as api from "../api";
import {
  IconBolt, IconTerminal, IconCode, IconBox, IconGlobe, IconWrench,
  IconChart, IconFolder, IconServer, IconClock, IconLayers, IconDatabase,
} from "./Icons";

// Background decoration only - every job-scheduler concept gets a spot:
// shell/script jobs, containers, HTTP jobs, cron, queues, workers, storage.
const FLOATERS = [
  { Icon: IconTerminal, top: "10%", left: "8%", size: 30, dur: 10, delay: 0 },
  { Icon: IconClock, top: "18%", left: "84%", size: 26, dur: 12, delay: 1.2 },
  { Icon: IconBox, top: "70%", left: "9%", size: 34, dur: 9, delay: 2.4 },
  { Icon: IconLayers, top: "78%", left: "88%", size: 28, dur: 11, delay: 0.6 },
  { Icon: IconGlobe, top: "6%", left: "46%", size: 24, dur: 13, delay: 3 },
  { Icon: IconCode, top: "42%", left: "4%", size: 22, dur: 8, delay: 1.8 },
  { Icon: IconServer, top: "50%", left: "93%", size: 30, dur: 10, delay: 0.9 },
  { Icon: IconDatabase, top: "88%", left: "48%", size: 26, dur: 12, delay: 2.1 },
  { Icon: IconWrench, top: "28%", left: "70%", size: 20, dur: 9, delay: 3.6 },
  { Icon: IconChart, top: "62%", left: "58%", size: 24, dur: 11, delay: 1.5 },
  { Icon: IconFolder, top: "86%", left: "20%", size: 22, dur: 10, delay: 2.8 },
  { Icon: IconBolt, top: "34%", left: "92%", size: 20, dur: 9, delay: 0.3 },
];

function FloatingIcons() {
  return (
    <div className="auth-floaters" aria-hidden="true">
      {FLOATERS.map(({ Icon, top, left, size, dur, delay }, i) => (
        <span
          key={i}
          className="auth-floater"
          style={{ top, left, "--size": `${size}px`, "--dur": `${dur}s`, "--delay": `${delay}s` }}
        >
          <Icon size={size} />
        </span>
      ))}
    </div>
  );
}

export default function AuthScreen({ onAuthed }) {
  const [mode, setMode] = useState("login"); // "login" | "register"
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);

  function switchMode(next) {
    setError("");
    setNotice("");
    setMode(next);
  }

  async function submit(e) {
    e.preventDefault();
    setError("");
    setNotice("");
    setBusy(true);
    try {
      if (mode === "login") {
        await api.login(email, password);
        onAuthed();
      } else {
        await api.register(email, password);
        // Registering issues a session (api.register calls setSession) -
        // drop it and send the user to the login form instead of signing
        // them in automatically.
        await api.logout();
        setPassword("");
        setMode("login");
        setNotice("Account created - sign in to continue.");
      }
    } catch (err) {
      setError(err.message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="auth-screen">
      <FloatingIcons />
      <div className="auth-glow auth-glow-1" />
      <div className="auth-glow auth-glow-2" />

      <form className="card auth-card" onSubmit={submit}>
        <div className="auth-brand">
          <span className="auth-brand-mark"><IconBolt size={22} /></span>
          <span className="brand">Job Scheduler</span>
        </div>
        <p className="auth-tagline">
          A distributed platform to programmatically create, schedule, and
          monitor background jobs across a fleet of workers.
        </p>

        <h1 className="auth-heading">{mode === "login" ? "Welcome back" : "Create your account"}</h1>

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

        {notice && <div className="notice">{notice}</div>}
        {error && <div className="error">{error}</div>}

        <button className="btn-primary auth-submit" type="submit" disabled={busy}>
          {busy ? "…" : mode === "login" ? "Sign in" : "Create account"}
        </button>

        <button
          type="button"
          className="link auth-switch"
          onClick={() => switchMode(mode === "login" ? "register" : "login")}
        >
          {mode === "login" ? "Need an account? Register" : "Have an account? Sign in"}
        </button>
      </form>
    </div>
  );
}
