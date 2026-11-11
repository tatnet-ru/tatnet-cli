package contract

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestComposePreservesSDKAndRawOperations(t *testing.T) {
	base := []byte(`{"openapi":"3.1.0","paths":{"/apps":{"get":{"operationId":"apps_list"}}},"components":{"schemas":{"App":{"type":"object"}}}}`)
	extra := []byte(`{"paths":{"/supabase":{"get":{"operationId":"supabase_list"}}},"components":{"schemas":{"Database":{"type":"object"}}}}`)
	merged, err := Compose(base, extra)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Paths      map[string]any
		Components struct{ Schemas map[string]any }
	}
	if err := json.Unmarshal(merged, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Paths) != 2 || len(doc.Components.Schemas) != 2 {
		t.Fatal("SDK or raw operation lost")
	}
	// Future SDK already containing the extension is a safe, repeatable sync.
	again, err := Compose(merged, extra)
	if err != nil || string(again) != string(merged) {
		t.Fatal("sync is not idempotent", err)
	}
}
func TestComposeRejectsConflictingPathsAndSchemas(t *testing.T) {
	base := []byte(`{"paths":{"/supabase":{"get":{}}},"components":{"schemas":{"Database":{"type":"object"}}}}`)
	for _, extra := range []string{
		`{"paths":{"/supabase":{"post":{}}},"components":{"schemas":{}}}`,
		`{"paths":{},"components":{"schemas":{"Database":{"type":"string"}}}}`,
	} {
		if _, err := Compose(base, []byte(extra)); err == nil || !strings.Contains(err.Error(), "conflict") {
			t.Fatal("conflict silently accepted", err)
		}
	}
}
