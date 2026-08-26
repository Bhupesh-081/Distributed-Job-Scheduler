-- Account customization: an optional display name shown in the UI instead
-- of the raw email. Nullable - falls back to email until the user sets one.
ALTER TABLE users ADD COLUMN IF NOT EXISTS display_name TEXT;
