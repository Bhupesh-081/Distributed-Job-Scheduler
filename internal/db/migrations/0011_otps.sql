-- Email verification + forgot/reset password, via a 6-digit one-time code
-- (OTP) instead of an emailed link - the dashboard has a real login screen
-- now, so a code the user types in beats a link with nowhere obvious to
-- land.
ALTER TABLE users ADD COLUMN IF NOT EXISTS email_verified BOOLEAN NOT NULL DEFAULT false;

CREATE TABLE IF NOT EXISTS otps (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    purpose    TEXT NOT NULL CHECK (purpose IN ('verify_email', 'reset_password')),
    -- SHA-256 of the code, never the code itself - same reasoning as
    -- refresh_tokens, even though a 6-digit code is inherently low-entropy
    -- (mitigated by a short expiry and a capped attempt count, not secrecy
    -- of the hash).
    code_hash  TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    attempts   INT NOT NULL DEFAULT 0,
    used_at    TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- store.VerifyOTP's "most recent active code for this user+purpose" query.
CREATE INDEX IF NOT EXISTS idx_otps_user_purpose ON otps(user_id, purpose);
