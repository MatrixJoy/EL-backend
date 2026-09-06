-- name: LookupDictionaryEntries :many
SELECT * FROM dictionary_entries
WHERE normalized_word = ANY(sqlc.arg('words')::text[])
ORDER BY
    array_position(sqlc.arg('words')::text[], normalized_word),
    sense_rank,
    part_of_speech,
    id
LIMIT 16;

-- name: ListDictionaryFormLemmas :many
SELECT lemma FROM dictionary_forms
WHERE form = $1
ORDER BY lemma;
