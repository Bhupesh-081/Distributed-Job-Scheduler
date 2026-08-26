import { useState } from "react";
import * as api from "../api";
import {
  IconBolt, IconTerminal, IconCode, IconBox, IconGlobe, IconWrench,
  IconChart, IconFolder, IconServer, IconClock, IconLayers, IconDatabase,
} from "./Icons";

// Background decoration only - every job-scheduler concept gets a spot:
// shell/script jobs, containers, HTTP jobs, cron, queues, workers, storage.
const FLOATERS = [
  { Icon: IconTerminal, top: "10%", left: "8%", size: 46, dur: 10, delay: 0 },
  { Icon: IconClock, top: "18%", left: "84%", size: 40, dur: 12, delay: 1.2 },
  { Icon: IconBox, top: "70%", left: "9%", size: 52, dur: 9, delay: 2.4 },
  { Icon: IconLayers, top: "78%", left: "88%", size: 44, dur: 11, delay: 0.6 },
  { Icon: IconGlobe, top: "6%", left: "46%", size: 38, dur: 13, delay: 3 },
  { Icon: IconCode, top: "42%", left: "4%", size: 34, dur: 8, delay: 1.8 },
  { Icon: IconServer, top: "50%", left: "93%", size: 46, dur: 10, delay: 0.9 },
  { Icon: IconDatabase, top: "88%", left: "48%", size: 40, dur: 12, delay: 2.1 },
  { Icon: IconWrench, top: "28%", left: "70%", size: 32, dur: 9, delay: 3.6 },
  { Icon: IconChart, top: "62%", left: "58%", size: 38, dur: 11, delay: 1.5 },
  { Icon: IconFolder, top: "86%", left: "20%", size: 34, dur: 10, delay: 2.8 },
  { Icon: IconBolt, top: "34%", left: "92%", size: 32, dur: 9, delay: 0.3 },
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

const COPY = {
  login: { heading: "Welcome back", sub: null },
  register: { heading: "Create your account", sub: null },
  verify: { heading: "Verify your email", sub: (email) => `Enter the 6-digit code we sent to ${email}` },
  forgot: { heading: "Forgot your password?", sub: () => "Enter your email and we'll send a reset code" },
  reset: { heading: "Reset your password", sub: (email) => `Enter the code we sent to ${email}` },
};

export default function AuthScreen({ onAuthed }) {
  const [mode, setMode] = useState("login"); // login | register | verify | forgot | reset
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [code, setCode] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);

  function switchMode(next) {
    setError("");
    setNotice("");
    setCode("");
    setMode(next);
  }

  async function resendCode(purpose) {
    setError("");
    setBusy(true);
    try {
      if (purpose === "verify") await api.resendVerificationOtp(email);
      else await api.forgotPassword(email);
      setNotice("Code sent - check your inbox.");
    } catch (err) {
      setError(err.message);
    } finally {
      setBusy(false);
    }
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
        return;
      }

      if (mode === "register") {
        // Registering issues a session, but we don't sign the user in yet
        // - they still need to prove they own this email address. Drop
        // it and move to the OTP step instead.
        await api.register(email, password);
        await api.logout();
        setPassword("");
        switchMode("verify");
        return;
      }

      if (mode === "verify") {
        await api.verifyEmailOtp(email, code);
        setCode("");
        switchMode("login");
        setNotice("Email verified - sign in to continue.");
        return;
      }

      if (mode === "forgot") {
        await api.forgotPassword(email);
        switchMode("reset");
        setNotice("If that email has an account, a reset code was sent.");
        return;
      }

      if (mode === "reset") {
        if (newPassword !== confirmPassword) {
          setError("Passwords don't match.");
          return;
        }
        await api.resetPasswordOtp(email, code, newPassword);
        setCode("");
        setNewPassword("");
        setConfirmPassword("");
        switchMode("login");
        setNotice("Password reset - sign in with your new password.");
      }
    } catch (err) {
      setError(err.message);
    } finally {
      setBusy(false);
    }
  }

  const copy = COPY[mode];
  const submitLabel = {
    login: "Sign in",
    register: "Create account",
    verify: "Verify email",
    forgot: "Send reset code",
    reset: "Reset password",
  }[mode];

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

        <h1 className="auth-heading">{copy.heading}</h1>
        {copy.sub && <p className="auth-subtext">{copy.sub(email)}</p>}

        {(mode === "login" || mode === "register" || mode === "forgot") && (
          <label>
            Email
            <input type="email" value={email} onChange={(e) => setEmail(e.target.value)} required autoFocus />
          </label>
        )}

        {(mode === "login" || mode === "register") && (
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
        )}

        {(mode === "verify" || mode === "reset") && (
          <label>
            6-digit code
            <input
              className="otp-input"
              inputMode="numeric"
              pattern="[0-9]*"
              maxLength={6}
              value={code}
              onChange={(e) => setCode(e.target.value.replace(/\D/g, "").slice(0, 6))}
              required
              autoFocus
            />
          </label>
        )}

        {mode === "reset" && (
          <>
            <label>
              New password
              <input
                type="password"
                value={newPassword}
                onChange={(e) => setNewPassword(e.target.value)}
                minLength={8}
                required
              />
            </label>
            <label>
              Confirm new password
              <input
                type="password"
                value={confirmPassword}
                onChange={(e) => setConfirmPassword(e.target.value)}
                minLength={8}
                required
              />
            </label>
          </>
        )}

        {notice && <div className="notice">{notice}</div>}
        {error && <div className="error">{error}</div>}

        <button className="btn-primary auth-submit" type="submit" disabled={busy}>
          {busy ? "…" : submitLabel}
        </button>

        {(mode === "verify" || mode === "reset") && (
          <button
            type="button"
            className="link auth-switch"
            onClick={() => resendCode(mode === "verify" ? "verify" : "reset")}
            disabled={busy}
          >
            Resend code
          </button>
        )}

        {mode === "login" && (
          <button type="button" className="link auth-switch" onClick={() => switchMode("forgot")}>
            Forgot password?
          </button>
        )}

        {(mode === "login" || mode === "register") && (
          <button
            type="button"
            className="link auth-switch"
            onClick={() => switchMode(mode === "login" ? "register" : "login")}
          >
            {mode === "login" ? "Need an account? Register" : "Have an account? Sign in"}
          </button>
        )}

        {(mode === "verify" || mode === "forgot" || mode === "reset") && (
          <button type="button" className="link auth-switch" onClick={() => switchMode("login")}>
            Back to sign in
          </button>
        )}
      </form>
    </div>
  );
}
