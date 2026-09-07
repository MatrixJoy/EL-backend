package api_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func TestOpenAPIContractIsValidAndComplete(t *testing.T) {
	path, err := filepath.Abs("openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	document, err := openapi3.NewLoader().LoadFromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := document.Validate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if document.Components.Schemas["GrammarPoint"] == nil || document.Components.Schemas["ContentDetailEnvelope"] == nil {
		t.Fatal("OpenAPI missing grammar-aware content schemas")
	}
	for _, route := range []string{"/bootstrap", "/home", "/categories", "/categories/{slug}/contents", "/contents", "/contents/{id}", "/search", "/sync", "/series", "/series/{id}", "/media/{id}", "/auth/apple", "/auth/development", "/me", "/me/bookmarks/{contentId}", "/me/progress/{contentId}", "/me/sync", "/me/retell-attempts", "/me/retell-attempts/{attemptId}", "/me/retell-attempts/{attemptId}/audio"} {
		if document.Paths.Find(route) == nil {
			t.Errorf("OpenAPI missing %s", route)
		}
	}
}
