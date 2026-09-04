CREATE TABLE cms_publications (
    idempotency_key text PRIMARY KEY,
    source_content_id text NOT NULL,
    source text NOT NULL,
    manifest_etag text NOT NULL,
    content_id uuid REFERENCES contents(id) ON DELETE SET NULL,
    content_revision integer,
    document jsonb NOT NULL,
    received_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX cms_publications_source_idx
    ON cms_publications (source, source_content_id, received_at DESC);
