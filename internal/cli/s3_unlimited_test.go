package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestS3CreateHasNoQuota(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/object-storage/buckets" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body["name"] != "unlimited-bucket" || body["quota_bytes"] != nil {
			t.Errorf("unexpected body %v", body)
		}
		writeJSON(w, map[string]any{"name": "unlimited-bucket", "quota_bytes": nil, "status": "active"})
	}))
	defer srv.Close()
	out, _, err := run(t, srv, "s3", "bucket", "create", "unlimited-bucket")
	if err != nil || !strings.Contains(out, "unlimited-bucket") {
		t.Fatalf("%s %v", out, err)
	}
	_, _, err = run(t, srv, "s3", "bucket", "create", "unlimited-bucket", "--quota-bytes", "1")
	if err == nil || !strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("retired flag accepted: %v", err)
	}
}
