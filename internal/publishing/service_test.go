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

func TestSlugify(t *testing.T) {
	if got := slugify("VOA: Everyday Grammar"); got != "voa-everyday-grammar" {
		t.Fatalf("slugify() = %q", got)
	}
}
