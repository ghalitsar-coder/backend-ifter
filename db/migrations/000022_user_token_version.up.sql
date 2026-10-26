-- Revocation counter for issued access tokens. Password reset/activation
-- increments it, making every previously issued access token fail
-- Authenticate even though the JWT signature is still valid.
ALTER TABLE users ADD COLUMN token_version integer NOT NULL DEFAULT 0;
