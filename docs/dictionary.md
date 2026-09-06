# Self-hosted dictionary

The public dictionary API is backed by PostgreSQL and does not depend on the iOS system dictionary or a runtime third-party API.

## Sources

1. Definitions curated in published learning content are inserted automatically and ranked first.
2. Princeton WordNet 3.1 supplies the general English lexicon, parts of speech, definitions, examples, and irregular morphology.

WordNet is distributed under the Princeton WordNet license. Keep the original archive and license notice with every deployed copy of the database, retain the `Princeton WordNet 3.1` attribution returned by the API, and follow the official license at <https://wordnet.princeton.edu/license-and-commercial-use>.

## Import

After deploying migrations and the API to the configured test host, run:

```bash
./scripts/import-wordnet.sh
```

The script downloads the official `wn3.1.dict.tar.gz` archive into a local cache, builds the repository importer for the remote host, copies both files to the deployment directory, and runs the importer inside the API container. Import is transactional and safe to repeat.

For a public-cloud deployment, run `cmd/dictionary-import` as a one-time job with `LEARNING_DATABASE_URL` and `-archive` rather than coupling the import to API startup.
