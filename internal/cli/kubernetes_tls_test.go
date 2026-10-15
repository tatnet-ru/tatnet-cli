package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Managed Kubernetes uses the existing generic API command. An older binary
// supports a newly deployed API endpoint through its documented version-skew flag.
func TestKubernetesTLSViaAPI(t *testing.T) {
	for _, code := range []int{200, 403, 409, 422} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "PATCH" || r.URL.Path != "/projects/p/kubernetes-clusters/c/settings" {
				t.Errorf("request %s %s", r.Method, r.URL.Path)
			}
			if r.Header.Get("Authorization") != "Bearer tn_live_тест" {
				t.Error("missing caller authentication")
			}
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Error(err)
			}
			if payload["tls_min_version"] != "VersionTLS13" {
				t.Errorf("payload %v", payload)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(code)
			if code == 200 {
				writeJSON(w, map[string]string{"id": "c", "tls_min_version": "VersionTLS13"})
			} else {
				writeJSON(w, map[string]string{"detail": "rejected"})
			}
		}))
		out, _, err := run(t, srv, "api", "/projects/p/kubernetes-clusters/c/settings", "-X", "PATCH", "-f", "tls_min_version=VersionTLS13", "--allow-unknown", "-o", "json")
		srv.Close()
		if code == 200 {
			if err != nil || !strings.Contains(out, "VersionTLS13") {
				t.Fatalf("%s, %v", out, err)
			}
		} else if err == nil || !strings.Contains(err.Error(), "rejected") {
			t.Fatalf("HTTP %d: %v", code, err)
		}
	}
}
