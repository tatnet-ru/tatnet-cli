package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestVpcRenameOnlySendsName(t *testing.T) {
	const id = "11111111-1111-1111-1111-111111111111"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PATCH" || r.URL.Path != "/vpcs/"+id || r.Header.Get("Authorization") == "" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if len(body) != 1 || body["name"] != "Новая сеть" {
			t.Errorf("unexpected body %v", body)
		}
		writeJSON(w, map[string]any{"id": id, "name": "Новая сеть", "subnet": "10.0.0.0/24"})
	}))
	defer srv.Close()
	out, _, err := run(t, srv, "vpc", "rename", id, "--name", "Новая сеть")
	if err != nil || !strings.Contains(out, "Новая сеть") {
		t.Fatalf("result %s %v", out, err)
	}
}
func TestVpcRenamePreservesPermissionError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		writeJSON(w, map[string]any{"detail": "Permission denied"})
	}))
	defer srv.Close()
	_, _, err := run(t, srv, "vpc", "rename", "11111111-1111-1111-1111-111111111111", "--name", "new")
	if err == nil || !strings.Contains(err.Error(), "Permission denied") {
		t.Fatalf("permission failure lost: %v", err)
	}
}

func TestDomainVerifyForwardsIdentifiers(t *testing.T) {
	const project = "11111111-1111-1111-1111-111111111111"
	const app = "22222222-2222-2222-2222-222222222222"
	const domain = "33333333-3333-3333-3333-333333333333"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == "/apps/"+app {
			writeJSON(w, map[string]any{"id": app, "project_id": project, "name": "test"})
			return
		}
		if r.Method != "POST" || r.URL.Path != "/projects/"+project+"/apps/"+app+"/domains/"+domain+"/verify" || r.Header.Get("Authorization") == "" {
			t.Errorf("unexpected verify request %s %s", r.Method, r.URL.Path)
		}
		writeJSON(w, map[string]any{"id": domain, "domain": "example.com", "status": "pending"})
	}))
	defer srv.Close()
	_, _, err := run(t, srv, "--project", project, "app", "domain", "verify", app, domain)
	if err != nil {
		t.Fatal(err)
	}
}
