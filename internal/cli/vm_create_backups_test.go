package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateVMIncludesBackupPolicyAtomically(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "off", true: "on"}[enabled], func(t *testing.T) {
			var body map[string]any
			posts := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/projects/"+observeProject {
					writeJSON(w, map[string]any{"id": observeProject, "name": "test"})
					return
				}
				if r.URL.Path == "/projects/"+observeProject+"/vms" && r.Method == "POST" {
					posts++
					if r.Header.Get("Authorization") == "" {
						t.Error("missing API authentication")
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					w.WriteHeader(201)
					writeJSON(w, map[string]any{"id": "44444444-4444-4444-8444-444444444444", "name": "test", "status": "pending"})
					return
				}
				t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				http.NotFound(w, r)
			}))
			defer srv.Close()
			args := []string{"-p", observeProject, "vm", "create", "--name", "test", "--image", "image", "--plan", "plan", "--cluster", "cluster", "--user", "smoke", "-o", "json"}
			if enabled {
				args = append(args, "--auto-backups", "--backup-hour-utc", "6", "--backup-retention-days", "14")
			}
			if _, _, err := run(t, srv, args...); err != nil {
				t.Fatal(err)
			}
			if posts != 1 {
				t.Fatalf("created %d VMs", posts)
			}
			if enabled {
				p := body["backup_policy"].(map[string]any)
				if p["enabled"] != true || p["hour_utc"] != float64(6) || p["retention_days"] != float64(14) {
					t.Fatalf("wrong policy: %v", p)
				}
			} else if _, ok := body["backup_policy"]; ok {
				t.Fatal("enabled backups without opt-in")
			}
		})
	}
}
