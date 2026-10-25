-- web-push (F9): browser Web Push subscriptions owned by a user. A user may
-- register several browsers/devices; the endpoint is unique, so re-subscribing
-- from the same browser refreshes the keys instead of duplicating the row.
CREATE TABLE push_subscriptions (
  id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  endpoint   text NOT NULL CHECK (btrim(endpoint) <> ''),
  p256dh     text NOT NULL CHECK (btrim(p256dh) <> ''),
  auth       text NOT NULL CHECK (btrim(auth) <> ''),
  user_agent varchar(500),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT push_subscriptions_user_agent_not_blank CHECK (user_agent IS NULL OR btrim(user_agent) <> '')
);

CREATE UNIQUE INDEX push_subscriptions_endpoint_key ON push_subscriptions (endpoint);
CREATE INDEX push_subscriptions_user_idx ON push_subscriptions (user_id, created_at DESC);
