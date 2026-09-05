package httpapi

import (
	"encoding/json"
	"testing"
)

func TestContentFeaturedWordsUsesStructuredMetadata(t *testing.T) {
	metadata := json.RawMessage(`{"featuredWords":[{"word":"pest","part_of_speech":"noun","definition":"an animal that causes problems"}]}`)
	words := contentFeaturedWords(metadata, json.RawMessage(`[]`))
	if len(words) != 1 || words[0].Word != "pest" || words[0].PartOfSpeech != "noun" {
		t.Fatalf("featured words = %#v", words)
	}
}

func TestContentFeaturedWordsSupportsLegacyVOABody(t *testing.T) {
	body := json.RawMessage(`[
		{"text":"Article paragraph."},
		{"text":"____________________"},
		{"text":"skull – n. the part of the head made of bone"},
		{"text":"hang out - phr v. to spend time together"},
		{"text":"____________________"},
		{"text":"Comment instructions"}
	]`)
	words := contentFeaturedWords(json.RawMessage(`{}`), body)
	if len(words) != 2 {
		t.Fatalf("featured words = %#v", words)
	}
	if words[1].Word != "hang out" || words[1].PartOfSpeech != "phrasal verb" || words[1].Definition != "to spend time together" {
		t.Fatalf("second featured word = %#v", words[1])
	}
}
