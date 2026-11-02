-- +goose Up
CREATE TABLE replication_meta (
    id smallint PRIMARY KEY CHECK(id=1),
    epoch uuid NOT NULL DEFAULT gen_random_uuid(),
    event_sequence bigint NOT NULL DEFAULT 0
);
INSERT INTO replication_meta(id) VALUES(1);
CREATE TABLE replication_roots (
    root_id text PRIMARY KEY,
    owner_id text NOT NULL,
    device_id text NOT NULL,
    signing_key text NOT NULL,
    encryption_key text NOT NULL,
    roster_version bigint NOT NULL DEFAULT 0,
    UNIQUE(owner_id,device_id)
);
CREATE TABLE replication_feeds (
    root_id text PRIMARY KEY REFERENCES replication_roots(root_id),
    sequence bigint NOT NULL DEFAULT 0
);
CREATE TABLE replication_peers (
    root_id text NOT NULL REFERENCES replication_roots(root_id),
    peer_root text NOT NULL,
    signing_key text NOT NULL,
    encryption_key text NOT NULL,
    active boolean NOT NULL,
    PRIMARY KEY(root_id,peer_root)
);
CREATE TABLE replication_links (
    left_root text NOT NULL,
    right_root text NOT NULL,
    generation bigint NOT NULL DEFAULT 0,
    active boolean NOT NULL DEFAULT false,
    PRIMARY KEY(left_root,right_root),
    CHECK(left_root<right_root)
);
CREATE TABLE replication_operations (
    root_id text NOT NULL REFERENCES replication_roots(root_id),
    operation_id uuid NOT NULL,
    payload_hash text NOT NULL,
    resulting_version bigint NOT NULL,
    PRIMARY KEY(root_id,operation_id)
);
CREATE TABLE replication_sources (
    root_id text NOT NULL REFERENCES replication_roots(root_id),
    stream_id uuid NOT NULL,
    uploaded_through bigint NOT NULL DEFAULT 0,
    PRIMARY KEY(root_id,stream_id)
);
CREATE TABLE replication_events (
    source_root text NOT NULL,
    stream_id uuid NOT NULL,
    event_id uuid NOT NULL,
    source_sequence bigint NOT NULL,
    stored_sequence bigint NOT NULL UNIQUE,
    aggregate_id uuid NOT NULL,
    aggregate_version bigint NOT NULL,
    authority text NOT NULL,
    kind text NOT NULL,
    peer_root text NOT NULL DEFAULT '',
    recipients jsonb NOT NULL,
    body bytea NOT NULL,
    payload_hash text NOT NULL,
    PRIMARY KEY(source_root,stream_id,event_id)
);
CREATE UNIQUE INDEX replication_client_sequences ON replication_events(source_root,stream_id,source_sequence) WHERE authority='LOCAL_DEVICE';
CREATE INDEX replication_events_peer ON replication_events(source_root,peer_root);
CREATE TABLE replication_backfills (
    left_root text NOT NULL,
    right_root text NOT NULL,
    generation bigint NOT NULL,
    upper_sequence bigint NOT NULL,
    after_sequence bigint NOT NULL DEFAULT 0,
    PRIMARY KEY(left_root,right_root,generation)
);
CREATE TABLE replication_deliveries (
    consumer_root text NOT NULL REFERENCES replication_roots(root_id),
    sequence bigint NOT NULL,
    source_root text NOT NULL,
    stream_id uuid NOT NULL,
    event_id uuid NOT NULL,
    link_left text,
    link_right text,
    link_generation bigint NOT NULL DEFAULT 0,
    PRIMARY KEY(consumer_root,sequence),
    UNIQUE(consumer_root,source_root,stream_id,event_id,link_generation),
    FOREIGN KEY(source_root,stream_id,event_id) REFERENCES replication_events(source_root,stream_id,event_id)
);
CREATE TABLE replication_business_audience (
    event_id uuid NOT NULL REFERENCES outbox_events(event_id),
    owner_id text NOT NULL,
    device_id text NOT NULL,
    routed_root text,
    PRIMARY KEY(event_id,owner_id,device_id)
);
CREATE INDEX replication_pending_audience ON replication_business_audience(owner_id,device_id) WHERE routed_root IS NULL;
ALTER TABLE outbox_events ADD COLUMN history_routed boolean NOT NULL DEFAULT false;
CREATE INDEX outbox_history_pending ON outbox_events(event_sequence) WHERE NOT history_routed;

-- +goose Down
DROP INDEX outbox_history_pending;
ALTER TABLE outbox_events DROP COLUMN history_routed;
DROP TABLE replication_business_audience;
DROP TABLE replication_deliveries;
DROP TABLE replication_backfills;
DROP TABLE replication_events;
DROP TABLE replication_sources;
DROP TABLE replication_operations;
DROP TABLE replication_links;
DROP TABLE replication_peers;
DROP TABLE replication_feeds;
DROP TABLE replication_roots;
DROP TABLE replication_meta;
