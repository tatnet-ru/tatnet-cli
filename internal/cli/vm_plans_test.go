package cli

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestVMPlansDiscovery(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/vms/plans" || r.URL.Query().Get("cluster_id") != "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb" || r.URL.Query().Get("role") != "k8s" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL)
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer tn_") {
			t.Error("missing API key")
		}
		writeJSON(w, map[string]any{"plans": []any{map[string]any{"id": "plan-4-8", "name": "Standard CPU", "vcpu": 4, "mem": 8192}}})
	}))
	defer srv.Close()
	out, _, err := run(t, srv, "vm", "plans", "--cluster", "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", "--role", "k8s", "-o", "json")
	if err != nil || !strings.Contains(out, "plan-4-8") || !strings.Contains(out, "8192") {
		t.Fatalf("out=%s err=%v", out, err)
	}
}

func TestVMPlansAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		_, _ = w.Write([]byte(`{"error":{"code":"forbidden","message":"role lacks vm:read"}}`))
	}))
	defer srv.Close()
	_, _, err := run(t, srv, "vm", "plans")
	if err == nil || !strings.Contains(err.Error(), "vm:read") {
		t.Fatalf("lost API refusal: %v", err)
	}
}
