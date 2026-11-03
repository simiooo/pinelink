-- +goose Up
CREATE TABLE accounts (
 id text PRIMARY KEY, contact_email text NOT NULL DEFAULT '', display_name text NOT NULL DEFAULT '',
 credential_epoch bigint NOT NULL DEFAULT 1, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE account_identities (
 issuer text NOT NULL, subject text NOT NULL, account_id text NOT NULL REFERENCES accounts(id),
 PRIMARY KEY(issuer,subject)
);
CREATE TABLE account_passwords (
 email text PRIMARY KEY, account_id text NOT NULL UNIQUE REFERENCES accounts(id),
 password_phc text NOT NULL, version bigint NOT NULL DEFAULT 1, updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE account_auth_requests (
 id uuid PRIMARY KEY, proof_key text NOT NULL, pkce_challenge text NOT NULL,
 surface text NOT NULL DEFAULT 'web', client_kind text NOT NULL CHECK(client_kind IN ('web','native')), browser_hash bytea,
 continuation text NOT NULL DEFAULT 'home', device_name text NOT NULL DEFAULT '',
 status text NOT NULL DEFAULT 'PENDING', account_id text REFERENCES accounts(id), credential_epoch bigint, ready_until timestamptz,
 provider_state_hash bytea UNIQUE, provider_nonce text, provider_verifier bytea,
 result_cipher bytea, claim_fingerprint bytea, claimed_until timestamptz,
 created_at timestamptz NOT NULL DEFAULT now(), expires_at timestamptz NOT NULL
);
CREATE TABLE account_sessions (
 id uuid PRIMARY KEY, token_hash bytea NOT NULL UNIQUE, account_id text NOT NULL REFERENCES accounts(id),
 device_id text NOT NULL, credential_epoch bigint NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 expires_at timestamptz NOT NULL, revoked_at timestamptz,
 FOREIGN KEY(account_id,device_id) REFERENCES devices(owner_id,device_id)
);
CREATE INDEX account_sessions_device ON account_sessions(account_id,device_id,expires_at) WHERE revoked_at IS NULL;
CREATE TABLE account_email_challenges (
 id uuid PRIMARY KEY, request_id uuid NOT NULL REFERENCES account_auth_requests(id),
 purpose text NOT NULL CHECK(purpose IN ('signup','reset')), email text NOT NULL,
 account_id text REFERENCES accounts(id), credential_version bigint, pending_password_phc text,
 generation integer NOT NULL DEFAULT 1, code_hash bytea NOT NULL, link_hash bytea NOT NULL UNIQUE,
 attempts integer NOT NULL DEFAULT 0, consumed_at timestamptz, reset_hash bytea,
 expires_at timestamptz NOT NULL, resend_at timestamptz NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX account_email_request_purpose ON account_email_challenges(request_id,purpose);
CREATE TABLE account_mail_outbox (
 id uuid PRIMARY KEY, challenge_id uuid NOT NULL REFERENCES account_email_challenges(id), generation integer NOT NULL,
 payload_cipher bytea NOT NULL, provider_id text, status text NOT NULL DEFAULT 'pending', attempts integer NOT NULL DEFAULT 0,
 lease_until timestamptz, next_attempt_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(challenge_id,generation)
);
CREATE TABLE account_rate_limits (
 key_hash bytea NOT NULL, window_start bigint NOT NULL, hits integer NOT NULL, expires_at timestamptz NOT NULL,
 PRIMARY KEY(key_hash,window_start)
);
ALTER TABLE peer_signals ADD COLUMN auth_session_id uuid REFERENCES account_sessions(id);
-- Existing owners and device keys are deliberately not assigned to accounts.

-- +goose Down
ALTER TABLE peer_signals DROP COLUMN auth_session_id;
DROP TABLE account_rate_limits, account_mail_outbox, account_email_challenges,
 account_sessions, account_auth_requests, account_passwords, account_identities, accounts;
