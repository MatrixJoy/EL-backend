package dictionary

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const SourceWordNet31 = "wordnet-3.1"

type ImportResult struct {
	Entries int64
	Forms   int64
}

type wordNetEntry struct {
	NormalizedWord string
	Word           string
	PartOfSpeech   string
	Definition     string
	Examples       []string
	SourceRef      string
}

func ImportWordNet31(ctx context.Context, pool *pgxpool.Pool, archive io.Reader) (ImportResult, error) {
	gzipReader, err := gzip.NewReader(archive)
	if err != nil {
		return ImportResult{}, fmt.Errorf("open WordNet gzip: %w", err)
	}
	defer gzipReader.Close()
	tx, err := pool.Begin(ctx)
	if err != nil {
		return ImportResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = tx.Exec(ctx, `CREATE TEMP TABLE dictionary_import_entries (
			normalized_word text, word text, part_of_speech text,
			definition text, examples text[], source_reference text
		) ON COMMIT DROP`); err != nil {
		return ImportResult{}, err
	}
	if _, err = tx.Exec(ctx, `CREATE TEMP TABLE dictionary_import_forms (
			form text, lemma text, part_of_speech text
		) ON COMMIT DROP`); err != nil {
		return ImportResult{}, err
	}
	if _, err = tx.Exec(ctx, `CREATE TEMP TABLE dictionary_import_ranks (
			normalized_word text, part_of_speech text,
			source_reference text, sense_rank integer
		) ON COMMIT DROP`); err != nil {
		return ImportResult{}, err
	}

	tarReader := tar.NewReader(gzipReader)
	found := make(map[string]bool)
	var result ImportResult
	for {
		header, nextErr := tarReader.Next()
		if nextErr == io.EOF {
			break
		}
		if nextErr != nil {
			return ImportResult{}, fmt.Errorf("read WordNet archive: %w", nextErr)
		}
		name := path.Base(header.Name)
		if partOfSpeech, ok := dataFilePartOfSpeech(name); ok {
			count, parseErr := importDataFile(ctx, tx, tarReader, partOfSpeech)
			if parseErr != nil {
				return ImportResult{}, fmt.Errorf("import %s: %w", name, parseErr)
			}
			result.Entries += count
			found[name] = true
			continue
		}
		if partOfSpeech, ok := exceptionFilePartOfSpeech(name); ok {
			count, parseErr := importExceptionFile(ctx, tx, tarReader, partOfSpeech)
			if parseErr != nil {
				return ImportResult{}, fmt.Errorf("import %s: %w", name, parseErr)
			}
			result.Forms += count
			found[name] = true
			continue
		}
		if partOfSpeech, ok := indexFilePartOfSpeech(name); ok {
			if parseErr := importIndexFile(ctx, tx, tarReader, partOfSpeech); parseErr != nil {
				return ImportResult{}, fmt.Errorf("import %s: %w", name, parseErr)
			}
			found[name] = true
		}
	}
	for _, required := range []string{"data.noun", "data.verb", "data.adj", "data.adv", "index.noun", "index.verb", "index.adj", "index.adv", "noun.exc", "verb.exc", "adj.exc", "adv.exc"} {
		if !found[required] {
			return ImportResult{}, fmt.Errorf("WordNet archive is missing %s", required)
		}
	}
	if _, err = tx.Exec(ctx, `DELETE FROM dictionary_entries WHERE source = $1`, SourceWordNet31); err != nil {
		return ImportResult{}, err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM dictionary_forms`); err != nil {
		return ImportResult{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO dictionary_entries (
			normalized_word, word, part_of_speech, definition, examples,
			source, source_reference, sense_rank
		)
		SELECT normalized_word, word, part_of_speech, definition, examples,
			$1, source_reference, sense_rank
		FROM (
			SELECT DISTINCT ON (entry.normalized_word, entry.part_of_speech, entry.definition)
				entry.*, COALESCE(rank.sense_rank, 100) AS sense_rank
			FROM dictionary_import_entries entry
			LEFT JOIN dictionary_import_ranks rank
			  ON rank.normalized_word = entry.normalized_word
			 AND rank.part_of_speech = entry.part_of_speech
			 AND rank.source_reference = entry.source_reference
			ORDER BY entry.normalized_word, entry.part_of_speech, entry.definition,
				COALESCE(rank.sense_rank, 100), entry.source_reference
		) entries
		ON CONFLICT (normalized_word, part_of_speech, definition, source) DO UPDATE
		SET word=excluded.word, examples=excluded.examples,
			source_reference=excluded.source_reference, updated_at=now()`, SourceWordNet31); err != nil {
		return ImportResult{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO dictionary_forms (form, lemma, part_of_speech)
		SELECT DISTINCT form, lemma, part_of_speech FROM dictionary_import_forms
		ON CONFLICT DO NOTHING`); err != nil {
		return ImportResult{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return ImportResult{}, err
	}
	return result, nil
}

func importDataFile(ctx context.Context, tx pgx.Tx, reader io.Reader, partOfSpeech string) (int64, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
	batch := make([][]any, 0, 4_000)
	var total int64
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		count, err := tx.CopyFrom(ctx, pgx.Identifier{"dictionary_import_entries"}, []string{"normalized_word", "word", "part_of_speech", "definition", "examples", "source_reference"}, pgx.CopyFromRows(batch))
		total += count
		batch = batch[:0]
		return err
	}
	for scanner.Scan() {
		entries, ok, err := parseDataLine(scanner.Text(), partOfSpeech)
		if err != nil {
			return total, err
		}
		if !ok {
			continue
		}
		for _, entry := range entries {
			batch = append(batch, []any{entry.NormalizedWord, entry.Word, entry.PartOfSpeech, entry.Definition, entry.Examples, entry.SourceRef})
			if len(batch) >= cap(batch) {
				if err := flush(); err != nil {
					return total, err
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return total, err
	}
	return total, flush()
}

func importExceptionFile(ctx context.Context, tx pgx.Tx, reader io.Reader, partOfSpeech string) (int64, error) {
	scanner := bufio.NewScanner(reader)
	batch := make([][]any, 0, 2_000)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		for _, lemma := range fields[1:] {
			batch = append(batch, []any{strings.ToLower(fields[0]), strings.ToLower(lemma), partOfSpeech})
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	return tx.CopyFrom(ctx, pgx.Identifier{"dictionary_import_forms"}, []string{"form", "lemma", "part_of_speech"}, pgx.CopyFromRows(batch))
}

func importIndexFile(ctx context.Context, tx pgx.Tx, reader io.Reader, partOfSpeech string) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
	batch := make([][]any, 0, 4_000)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		_, err := tx.CopyFrom(ctx, pgx.Identifier{"dictionary_import_ranks"}, []string{"normalized_word", "part_of_speech", "source_reference", "sense_rank"}, pgx.CopyFromRows(batch))
		batch = batch[:0]
		return err
	}
	for scanner.Scan() {
		rows, ok, err := parseIndexLine(scanner.Text(), partOfSpeech)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		for _, row := range rows {
			batch = append(batch, row)
			if len(batch) >= cap(batch) {
				if err := flush(); err != nil {
					return err
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return flush()
}

func parseIndexLine(line, partOfSpeech string) ([][]any, bool, error) {
	if strings.HasPrefix(line, "  ") || strings.TrimSpace(line) == "" {
		return nil, false, nil
	}
	fields := strings.Fields(line)
	if len(fields) < 6 {
		return nil, false, fmt.Errorf("invalid index entry")
	}
	synsetCount, err := strconv.Atoi(fields[2])
	if err != nil {
		return nil, false, fmt.Errorf("invalid index synset count")
	}
	pointerCount, err := strconv.Atoi(fields[3])
	if err != nil {
		return nil, false, fmt.Errorf("invalid index pointer count")
	}
	offsetStart := 6 + pointerCount
	if synsetCount < 1 || len(fields) < offsetStart+synsetCount {
		return nil, false, fmt.Errorf("invalid index offsets")
	}
	word := strings.ToLower(stripAdjectiveMarker(fields[0]))
	rows := make([][]any, 0, synsetCount)
	for index := 0; index < synsetCount; index++ {
		rows = append(rows, []any{word, partOfSpeech, partOfSpeechCode(partOfSpeech) + ":" + fields[offsetStart+index], index + 1})
	}
	return rows, true, nil
}

func parseDataLine(line, partOfSpeech string) ([]wordNetEntry, bool, error) {
	if strings.HasPrefix(line, "  ") || strings.TrimSpace(line) == "" {
		return nil, false, nil
	}
	sections := strings.SplitN(line, " | ", 2)
	if len(sections) != 2 {
		return nil, false, fmt.Errorf("missing gloss separator")
	}
	fields := strings.Fields(sections[0])
	if len(fields) < 4 {
		return nil, false, fmt.Errorf("invalid synset header")
	}
	wordCount, err := strconv.ParseInt(fields[3], 16, 32)
	if err != nil || len(fields) < 4+int(wordCount)*2 {
		return nil, false, fmt.Errorf("invalid synset word count")
	}
	definition, examples := parseGloss(sections[1])
	if definition == "" {
		return nil, false, nil
	}
	entries := make([]wordNetEntry, 0, wordCount)
	for index := 0; index < int(wordCount); index++ {
		word := stripAdjectiveMarker(fields[4+index*2])
		entries = append(entries, wordNetEntry{
			NormalizedWord: strings.ToLower(word),
			Word:           word,
			PartOfSpeech:   partOfSpeech,
			Definition:     definition,
			Examples:       examples,
			SourceRef:      partOfSpeechCode(partOfSpeech) + ":" + fields[0],
		})
	}
	return entries, true, nil
}

func parseGloss(gloss string) (string, []string) {
	parts := strings.Split(strings.TrimSpace(gloss), "; ")
	definitions := make([]string, 0, len(parts))
	examples := make([]string, 0, 2)
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if len(part) >= 2 && strings.HasPrefix(part, `"`) && strings.HasSuffix(part, `"`) {
			examples = append(examples, strings.Trim(part, `"`))
			continue
		}
		definitions = append(definitions, part)
	}
	return strings.Join(definitions, "; "), examples
}

func stripAdjectiveMarker(word string) string {
	for _, suffix := range []string{"(a)", "(p)", "(ip)"} {
		word = strings.TrimSuffix(word, suffix)
	}
	return word
}

func dataFilePartOfSpeech(name string) (string, bool) {
	values := map[string]string{"data.noun": "noun", "data.verb": "verb", "data.adj": "adjective", "data.adv": "adverb"}
	value, ok := values[name]
	return value, ok
}

func exceptionFilePartOfSpeech(name string) (string, bool) {
	values := map[string]string{"noun.exc": "noun", "verb.exc": "verb", "adj.exc": "adjective", "adv.exc": "adverb"}
	value, ok := values[name]
	return value, ok
}

func indexFilePartOfSpeech(name string) (string, bool) {
	values := map[string]string{"index.noun": "noun", "index.verb": "verb", "index.adj": "adjective", "index.adv": "adverb"}
	value, ok := values[name]
	return value, ok
}

func partOfSpeechCode(partOfSpeech string) string {
	switch partOfSpeech {
	case "noun":
		return "n"
	case "verb":
		return "v"
	case "adjective":
		return "a"
	case "adverb":
		return "r"
	default:
		return "x"
	}
}
