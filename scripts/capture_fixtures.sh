#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
backend_dir=$(CDPATH= cd -- "$script_dir/.." && pwd)
manifest="$backend_dir/fixtures/voa/manifest.txt"
output_dir="$backend_dir/fixtures/voa/live"
mkdir -p "$output_dir"

while IFS='|' read -r name source_url; do
    case "$name" in
        ''|'#'*) continue ;;
    esac
    echo "capturing $name"
    curl --fail --silent --show-error --location \
        --max-time 30 \
        --user-agent 'VOALearningApp-FixtureCapture/0.1' \
        --output "$output_dir/$name.html" \
        "$source_url"
done < "$manifest"

find "$output_dir" -name '*.html' -type f -size +0c | sort
