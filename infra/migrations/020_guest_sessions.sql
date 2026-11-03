-- +goose Up
CREATE TABLE guest_identities (
 owner_id text PRIMARY KEY, device_id text NOT NULL, signing_key text NOT NULL UNIQUE,
 encryption_key text NOT NULL, display_name text NOT NULL, client_kind text NOT NULL CHECK(client_kind IN ('web','native')),
 surface text NOT NULL CHECK(surface IN ('web','app')), current_session_id uuid,
 created_at timestamptz NOT NULL DEFAULT now(), revoked_at timestamptz,
 FOREIGN KEY(owner_id,device_id) REFERENCES devices(owner_id,device_id)
);
CREATE TABLE guest_sessions (
 id uuid PRIMARY KEY, owner_id text NOT NULL REFERENCES guest_identities(owner_id), token_hash bytea NOT NULL UNIQUE,
 expires_at timestamptz NOT NULL, revoked_at timestamptz, created_at timestamptz NOT NULL DEFAULT now()
);
ALTER TABLE guest_identities ADD CONSTRAINT guest_current_session FOREIGN KEY(current_session_id) REFERENCES guest_sessions(id);
CREATE TABLE guest_challenges (
 id uuid PRIMARY KEY, signing_key text NOT NULL, client_kind text NOT NULL CHECK(client_kind IN ('web','native')),
 surface text NOT NULL CHECK(surface IN ('web','app')), expires_at timestamptz NOT NULL,
 consumed_at timestamptz, body_hash bytea, session_id uuid REFERENCES guest_sessions(id)
);
CREATE INDEX guest_challenges_expiry ON guest_challenges(expires_at);
CREATE TABLE guest_invites (
 id uuid PRIMARY KEY, token_hash bytea NOT NULL UNIQUE, code_hash bytea NOT NULL UNIQUE,
 owner_id text NOT NULL, device_id text NOT NULL, expires_at timestamptz NOT NULL,
 consumed_at timestamptz, revoked_at timestamptz, selected_candidate uuid,
 created_at timestamptz NOT NULL DEFAULT now(), FOREIGN KEY(owner_id,device_id) REFERENCES devices(owner_id,device_id)
);
CREATE TABLE guest_pair_candidates (
 id uuid PRIMARY KEY, invite_id uuid NOT NULL REFERENCES guest_invites(id) ON DELETE CASCADE,
 owner_id text NOT NULL, device_id text NOT NULL, status text NOT NULL DEFAULT 'WAITING' CHECK(status IN ('WAITING','APPROVED','DECLINED')),
 created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(invite_id,owner_id,device_id),
 FOREIGN KEY(owner_id,device_id) REFERENCES devices(owner_id,device_id)
);
CREATE TABLE guest_pair_grants (
 id uuid PRIMARY KEY, invite_id uuid NOT NULL UNIQUE REFERENCES guest_invites(id),
 owner_a text NOT NULL, device_a text NOT NULL, owner_b text NOT NULL, device_b text NOT NULL,
 expires_at timestamptz NOT NULL, revoked_at timestamptz,
 FOREIGN KEY(owner_a,device_a) REFERENCES devices(owner_id,device_id), FOREIGN KEY(owner_b,device_b) REFERENCES devices(owner_id,device_id),
 CHECK(owner_a<>owner_b OR device_a<>device_b)
);
CREATE INDEX guest_pair_owner_a ON guest_pair_grants(owner_a,device_a);
CREATE INDEX guest_pair_owner_b ON guest_pair_grants(owner_b,device_b);
CREATE TABLE guest_resource_budgets (
 key_hash bytea NOT NULL, period bigint NOT NULL, bytes_reserved bigint NOT NULL CHECK(bytes_reserved>=0),
 expires_at timestamptz NOT NULL, PRIMARY KEY(key_hash,period)
);
CREATE TABLE relay_browser_tickets (
 token_hash bytea PRIMARY KEY, transfer_id uuid NOT NULL REFERENCES transfer_requests(id),
 owner_id text NOT NULL, device_id text NOT NULL, role text NOT NULL CHECK(role IN ('sender','receiver')),
 account_session_id uuid REFERENCES account_sessions(id), guest_session_id uuid REFERENCES guest_sessions(id),
 resume_from bigint CHECK(resume_from>=0),
 expires_at timestamptz NOT NULL, consumed_at timestamptz,
 CHECK((account_session_id IS NOT NULL)::int+(guest_session_id IS NOT NULL)::int=1)
);
ALTER TABLE transfer_requests ADD COLUMN guest_pair_id uuid REFERENCES guest_pair_grants(id);
ALTER TABLE transfer_requests ADD COLUMN client_request_id uuid;
CREATE UNIQUE INDEX transfer_client_request ON transfer_requests(sender_owner_id,sender_device_id,client_request_id) WHERE client_request_id IS NOT NULL;
ALTER TABLE peer_signals ADD COLUMN guest_session_id uuid REFERENCES guest_sessions(id);
ALTER TABLE peer_signals ADD CONSTRAINT peer_signal_session_kind CHECK(auth_session_id IS NULL OR guest_session_id IS NULL);

-- +goose Down
ALTER TABLE peer_signals DROP CONSTRAINT peer_signal_session_kind;
ALTER TABLE peer_signals DROP COLUMN guest_session_id;
ALTER TABLE transfer_requests DROP COLUMN guest_pair_id;
ALTER TABLE transfer_requests DROP COLUMN client_request_id;
DROP TABLE relay_browser_tickets, guest_resource_budgets, guest_pair_grants, guest_pair_candidates, guest_invites, guest_challenges;
ALTER TABLE guest_identities DROP CONSTRAINT guest_current_session;
DROP TABLE guest_sessions, guest_identities;
