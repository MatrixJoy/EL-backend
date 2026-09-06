CREATE TABLE dictionary_entries (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    normalized_word text NOT NULL,
    word text NOT NULL,
    part_of_speech text NOT NULL,
    definition text NOT NULL,
    examples text[] NOT NULL DEFAULT '{}',
    source text NOT NULL,
    source_reference text NOT NULL DEFAULT '',
    sense_rank integer NOT NULL DEFAULT 100,
    content_id uuid REFERENCES contents(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (normalized_word, part_of_speech, definition, source)
);

CREATE INDEX dictionary_entries_word_idx
    ON dictionary_entries (normalized_word, sense_rank, part_of_speech);

CREATE TABLE dictionary_forms (
    form text NOT NULL,
    lemma text NOT NULL,
    part_of_speech text NOT NULL,
    PRIMARY KEY (form, lemma, part_of_speech)
);

INSERT INTO dictionary_entries (
    normalized_word, word, part_of_speech, definition,
    source, source_reference, sense_rank, content_id
)
SELECT DISTINCT ON (
    lower(trim(featured->>'word')),
    coalesce(nullif(trim(featured->>'part_of_speech'), ''), 'other'),
    trim(featured->>'definition')
)
    lower(trim(featured->>'word')),
    trim(featured->>'word'),
    coalesce(nullif(trim(featured->>'part_of_speech'), ''), 'other'),
    trim(featured->>'definition'),
    'published-content',
    coalesce(c.metadata->>'sourceContentId', c.id::text),
    0,
    c.id
FROM contents c
CROSS JOIN LATERAL jsonb_array_elements(
    CASE
        WHEN jsonb_typeof(c.metadata->'featuredWords') = 'array' THEN c.metadata->'featuredWords'
        ELSE '[]'::jsonb
    END
) AS featured
WHERE c.status = 'published'
  AND trim(featured->>'word') <> ''
  AND trim(featured->>'definition') <> ''
ORDER BY
    lower(trim(featured->>'word')),
    coalesce(nullif(trim(featured->>'part_of_speech'), ''), 'other'),
    trim(featured->>'definition'),
    c.updated_at DESC;
