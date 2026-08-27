import { useEffect, useState } from "react";
import * as api from "../api";
import { fmtDate } from "../format";

export default function Account({ onLogout }) {
  const [me, setMe] = useState(null);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");

  const [displayName, setDisplayName] = useState("");
  const [savingName, setSavingName] = useState(false);

  const [code, setCode] = useState("");
  const [verifyBusy, setVerifyBusy] = useState(false);

  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [pwBusy, setPwBusy] = useState(false);

  async function refresh() {
    try {
      const data = await api.getMe();
      setMe(data);
      setDisplayName(data.display_name || "");
    } catch (err) {
      setError(err.message);
    }
  }

  useEffect(() => {
    refresh();
  }, []);

  async function saveDisplayName(e) {
    e.preventDefault();
    setError("");
    setNotice("");
    setSavingName(true);
    try {
      await api.updateDisplayName(displayName.trim());
      await refresh();
      setNotice("Display name saved.");
    } catch (err) {
      setError(err.message);
    } finally {
      setSavingName(false);
    }
  }

  async function verify(e) {
    e.preventDefault();
    setError("");
    setNotice("");
    setVerifyBusy(true);
    try {
      await api.verifyEmailOtp(me.email, code);
      setCode("");
      await refresh();
      setNotice("Email verified.");
    } catch (err) {
      setError(err.message);
    } finally {
      setVerifyBusy(false);
    }
  }

  async function resendCode() {
    setError("");
    setNotice("");
    try {
      await api.resendVerificationOtp(me.email);
      setNotice("Code sent - check your inbox.");
    } catch (err) {
      setError(err.message);
    }
  }

  async function submitPasswordChange(e) {
    e.preventDefault();
    setError("");
    setNotice("");
    if (newPassword !== confirmPassword) {
      setError("Passwords don't match.");
      return;
    }
    setPwBusy(true);
    try {
      await api.changePassword(currentPassword, newPassword);
      // Every session was just revoked server-side, including this one.
      onLogout();
    } catch (err) {
      setError(err.message);
      setPwBusy(false);
    }
  }

  if (error && !me) return <div className="error">{error}</div>;
  if (!me) return <p className="muted">Loading…</p>;

  return (
    <div>
      <div className="content-header"><h2>Account</h2></div>

      {notice && <div className="notice">{notice}</div>}
      {error && <div className="error">{error}</div>}

      <div className="card settings-card">
        <h3>Profile</h3>
        <div className="settings-row">
          <span>Email</span>
          <span className="mono">{me.email}</span>
        </div>
        <div className="settings-row">
          <span>Status</span>
          <span className={`pill ${me.email_verified ? "pill-good" : "pill-warn"}`}>
            {me.email_verified ? "Verified" : "Not verified"}
          </span>
        </div>
        <div className="settings-row">
          <span>Member since</span>
          <span>{fmtDate(me.created_at)}</span>
        </div>

        <form className="account-name-form" onSubmit={saveDisplayName}>
          <label>
            Display name
            <input
              placeholder={me.email}
              value={displayName}
              onChange={(e) => setDisplayName(e.target.value)}
              maxLength={60}
            />
          </label>
          <button className="btn-primary" type="submit" disabled={savingName}>Save</button>
        </form>
      </div>

      {!me.email_verified && (
        <div className="card settings-card">
          <h3>Verify your email</h3>
          <p className="muted">Enter the 6-digit code sent to {me.email}.</p>
          <form className="account-verify-form" onSubmit={verify}>
            <input
              className="otp-input"
              inputMode="numeric"
              pattern="[0-9]*"
              maxLength={6}
              value={code}
              onChange={(e) => setCode(e.target.value.replace(/\D/g, "").slice(0, 6))}
              required
            />
            <button className="btn-primary" type="submit" disabled={verifyBusy}>Verify</button>
          </form>
          <button type="button" className="link" onClick={resendCode}>Resend code</button>
        </div>
      )}

      <div className="card settings-card">
        <h3>Change password</h3>
        <form className="stacked-form" onSubmit={submitPasswordChange}>
          <label>
            Current password
            <input type="password" value={currentPassword} onChange={(e) => setCurrentPassword(e.target.value)} minLength={8} required />
          </label>
          <label>
            New password
            <input type="password" value={newPassword} onChange={(e) => setNewPassword(e.target.value)} minLength={8} required />
          </label>
          <label>
            Confirm new password
            <input type="password" value={confirmPassword} onChange={(e) => setConfirmPassword(e.target.value)} minLength={8} required />
          </label>
          <button className="btn-primary" type="submit" disabled={pwBusy}>Change password</button>
        </form>
        <p className="muted settings-hint">Changing your password signs you out everywhere, including here.</p>
      </div>
    </div>
  );
}
