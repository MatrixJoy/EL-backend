CREATE TABLE categories (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    slug text NOT NULL UNIQUE,
    name text NOT NULL,
    kind text NOT NULL DEFAULT 'level',
    parent_id uuid REFERENCES categories(id) ON DELETE SET NULL,
    sort_order integer NOT NULL DEFAULT 0,
    status text NOT NULL DEFAULT 'active'
);

CREATE TABLE series (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    slug text NOT NULL UNIQUE,
    title text NOT NULL,
    description text,
    level text,
    source_item_id uuid REFERENCES source_items(id) ON DELETE SET NULL,
    status text NOT NULL DEFAULT 'active',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE content_categories (
    content_id uuid NOT NULL REFERENCES contents(id) ON DELETE CASCADE,
    category_id uuid NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
    is_primary boolean NOT NULL DEFAULT false,
    position integer NOT NULL DEFAULT 0,
    PRIMARY KEY (content_id, category_id)
);

CREATE TABLE content_series (
    content_id uuid PRIMARY KEY REFERENCES contents(id) ON DELETE CASCADE,
    series_id uuid NOT NULL REFERENCES series(id) ON DELETE CASCADE,
    position integer NOT NULL DEFAULT 0
);

CREATE INDEX contents_search_idx ON contents USING gin (
    to_tsvector('english', coalesce(title, '') || ' ' || coalesce(summary, ''))
);
CREATE INDEX publish_events_entity_idx ON publish_events (entity_type, sequence);

INSERT INTO categories (slug, name, sort_order) VALUES
    ('beginning', 'Beginning', 10),
    ('intermediate', 'Intermediate', 20),
    ('advanced', 'Advanced', 30)
ON CONFLICT (slug) DO NOTHING;
