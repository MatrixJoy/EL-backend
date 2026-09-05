DROP INDEX IF EXISTS contents_search_idx;

CREATE INDEX contents_search_idx ON contents USING gin (
    to_tsvector(
        'english',
        coalesce(title, '') || ' ' ||
        coalesce(summary, '') || ' ' ||
        coalesce(body_blocks::text, '') || ' ' ||
        coalesce(metadata::text, '')
    )
);
