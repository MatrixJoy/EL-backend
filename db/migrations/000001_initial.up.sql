CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TYPE source_fetch_state AS ENUM ('discovered', 'scheduled', 'fetched', 'failed', 'disabled');
CREATE TYPE content_status AS ENUM ('draft', 'review', 'published', 'hidden', 'removed');
CREATE TYPE content_type AS ENUM ('article', 'lesson', 'podcast', 'video', 'quiz');
CREATE TYPE delivery_policy AS ENUM ('stream_proxy', 'redirect', 'managed_cache', 'source_page_only');
CREATE TYPE rights_status AS ENUM ('unknown', 'review_required', 'public_domain_verified', 'third_party_restricted');

CREATE TABLE source_items (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    source text NOT NULL,
    canonical_url text NOT NULL,
    external_id text,
    page_type text,
    first_seen_at timestamptz NOT NULL DEFAULT now(),
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    fetch_state source_fetch_state NOT NULL DEFAULT 'discovered',
    next_fetch_at timestamptz,
    UNIQUE (source, canonical_url)
);

CREATE TABLE source_snapshots (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    source_item_id uuid NOT NULL REFERENCES source_items(id) ON DELETE CASCADE,
    fetched_at timestamptz NOT NULL DEFAULT now(),
    http_status integer NOT NULL,
    etag text,
    last_modified text,
    content_hash text NOT NULL,
    object_key text NOT NULL,
    parser_version text,
    parse_state text NOT NULL DEFAULT 'pending',
    error_code text,
    UNIQUE (source_item_id, content_hash)
);

CREATE TABLE contents (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    source_item_id uuid NOT NULL UNIQUE REFERENCES source_items(id) ON DELETE RESTRICT,
    slug text NOT NULL UNIQUE,
    type content_type NOT NULL,
    title text NOT NULL,
    summary text,
    level text,
    published_at timestamptz,
    duration_seconds integer,
    body_blocks jsonb NOT NULL DEFAULT '[]'::jsonb,
    transcript_blocks jsonb NOT NULL DEFAULT '[]'::jsonb,
    metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    rights_status rights_status NOT NULL DEFAULT 'unknown',
    attribution text NOT NULL DEFAULT 'VOA Learning English',
    status content_status NOT NULL DEFAULT 'draft',
    revision integer NOT NULL DEFAULT 1 CHECK (revision > 0),
    source_updated_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX contents_feed_idx ON contents (status, published_at DESC, id);

CREATE TABLE assets (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    kind text NOT NULL,
    source_url text NOT NULL UNIQUE,
    source_host text NOT NULL,
    mime_type text,
    byte_size bigint CHECK (byte_size IS NULL OR byte_size >= 0),
    duration_seconds integer CHECK (duration_seconds IS NULL OR duration_seconds >= 0),
    width integer,
    height integer,
    quality_label text,
    checksum text,
    delivery_policy delivery_policy NOT NULL DEFAULT 'stream_proxy',
    rights_status rights_status NOT NULL DEFAULT 'unknown',
    attribution text NOT NULL DEFAULT 'VOA Learning English',
    object_key text,
    availability text NOT NULL DEFAULT 'available',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE content_assets (
    content_id uuid NOT NULL REFERENCES contents(id) ON DELETE CASCADE,
    asset_id uuid NOT NULL REFERENCES assets(id) ON DELETE RESTRICT,
    role text NOT NULL,
    position integer NOT NULL DEFAULT 0,
    PRIMARY KEY (content_id, asset_id, role)
);

CREATE TABLE publish_events (
    sequence bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    entity_type text NOT NULL,
    entity_id uuid NOT NULL,
    operation text NOT NULL CHECK (operation IN ('upsert', 'delete')),
    revision integer,
    published_at timestamptz NOT NULL DEFAULT now()
);
