package dictionary

import (
	"reflect"
	"testing"
)

func TestParseDataLine(t *testing.T) {
	t.Parallel()
	line := `00001740 03 n 02 entity 0 physical_entity 0 000 | that which is perceived or known; "everything is an entity"`
	entries, ok, err := parseDataLine(line, "noun")
	if err != nil || !ok {
		t.Fatalf("parseDataLine() ok=%v err=%v", ok, err)
	}
	if len(entries) != 2 || entries[0].Word != "entity" || entries[1].Word != "physical_entity" {
		t.Fatalf("parseDataLine() entries=%+v", entries)
	}
	if entries[0].Definition != "that which is perceived or known" || !reflect.DeepEqual(entries[0].Examples, []string{"everything is an entity"}) {
		t.Fatalf("parseDataLine() first=%+v", entries[0])
	}
}

func TestParseGlossPreservesDefinitionSemicolons(t *testing.T) {
	t.Parallel()
	definition, examples := parseGloss(`first clause; second clause; "an example"`)
	if definition != "first clause; second clause" || !reflect.DeepEqual(examples, []string{"an example"}) {
		t.Fatalf("parseGloss() definition=%q examples=%v", definition, examples)
	}
}

func TestStripAdjectiveMarker(t *testing.T) {
	t.Parallel()
	if got := stripAdjectiveMarker("ready(p)"); got != "ready" {
		t.Fatalf("stripAdjectiveMarker()=%q", got)
	}
}
