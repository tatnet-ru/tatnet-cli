package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestVMBackupPolicyPreservesUnchangedFields(t *testing.T) {
	vm := "44444444-4444-4444-8444-444444444444"
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			t.Error("missing API key")
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/backup-policy"):
			if r.Method == "PUT" {
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
			}
			writeJSON(w, map[string]any{"enabled": true, "hour_utc": 3, "retention_days": 7})
		case strings.HasPrefix(r.URL.Path, "/vms/"):
			writeJSON(w, map[string]any{"id": vm, "project_id": observeProject, "name": "backup-vm"})
		case r.URL.Path == "/projects/"+observeProject:
			writeJSON(w, map[string]any{"id": observeProject, "name": "test"})
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	_, _, err := run(t, srv, "-p", observeProject, "vm", "backup", "policy", vm, "--retention-days", "14", "-o", "json")
	if err != nil {
		t.Fatal(err)
	}
	if body["enabled"] != true || body["hour_utc"] != float64(3) || body["retention_days"] != float64(14) {
		t.Fatalf("unexpected policy: %v", body)
	}
}
