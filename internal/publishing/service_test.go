package publishing

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func validDocument() Document {
	checksum := sha256.Sum256([]byte("audio"))
	start, end := 0, 1200
	return Document{
		SchemaVersion: 1, SourceContentID: "voa-123", Source: "voa-learning-english", SourceURL: "https://learningenglish.voanews.com/a/123.html", ManifestETag: "etag", Title: "Learning English",
		Paragraphs: []Paragraph{{Index: 0, Text: "Learning English is useful."}},
		Timeline:   Timeline{Version: "base.en-v2", Sentences: []Sentence{{Text: "Learning English is useful.", StartMS: &start, EndMS: &end, Spoken: true}}},
		Audio:      Audio{SHA256: hex.EncodeToString(checksum[:]), ByteSize: 5, MIMEType: "audio/mpeg"},
	}
}

func TestValidatePublication(t *testing.T) {
	document := validDocument()
	if err := validate("12345678901234567890123456789012", document); err != nil {
		t.Fatalf("valid document rejected: %v", err)
	}
	document.Audio.SHA256 = "not-a-checksum"
	if err := validate("12345678901234567890123456789012", document); err == nil {
		t.Fatal("invalid checksum accepted")
	}
}

func TestValidateFeaturedWords(t *testing.T) {
	document := validDocument()
	document.SchemaVersion = 2
	document.FeaturedWords = []FeaturedWord{{Word: "pest", PartOfSpeech: "noun", Definition: "an animal or insect that causes problems"}}
	if err := validate("12345678901234567890123456789012", document); err != nil {
		t.Fatalf("valid featured word rejected: %v", err)
	}
	document.FeaturedWords[0].Definition = ""
	if err := validate("12345678901234567890123456789012", document); err == nil {
		t.Fatal("featured word without a definition accepted")
	}
}

func TestValidateGrammarPoints(t *testing.T) {
	document := validDocument()
	document.SchemaVersion = 2
	document.GrammarVersion = 1
	document.GrammarPoints = []GrammarPoint{{
		Kind: "modal", Title: "Modal verb", Explanation: "A modal expresses possibility.",
		Example: "People can learn the process.", Prompt: "People _____ learn the process.",
		Answer: "can", Options: []string{"can", "could", "might"},
	}}
	if err := validate("12345678901234567890123456789012", document); err != nil {
		t.Fatalf("valid grammar point rejected: %v", err)
	}
	document.GrammarPoints[0].Answer = "must"
	if err := validate("12345678901234567890123456789012", document); err == nil {
		t.Fatal("grammar answer outside options accepted")
	}
	document.GrammarPoints[0].Answer = "can"
	document.GrammarPoints[0].Prompt = document.GrammarPoints[0].Example
	if err := validate("12345678901234567890123456789012", document); err == nil {
		t.Fatal("grammar point without one blank accepted")
	}
}

func TestSlugify(t *testing.T) {
	if got := slugify("VOA: Everyday Grammar"); got != "voa-everyday-grammar" {
		t.Fatalf("slugify() = %q", got)
	}
}
