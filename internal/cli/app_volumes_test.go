package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAPIAppVolumeAttachUsesExistingPublicEndpoint(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.Method != "POST" || r.URL.Path != "/projects/p/volumes/v/attach" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body["app_id"] != "app" || body["mount_path"] != "/data" || body["vm_id"] != nil {
			t.Errorf("bad body: %+v", body)
		}
		writeJSON(w, map[string]any{"id": "v", "attachment": map[string]any{"app_id": "app", "mount_path": "/data", "status": "attaching"}})
	}))
	defer srv.Close()
	_, _, err := run(t, srv, "api", "/projects/p/volumes/v/attach", "-X", "POST", "-f", "app_id=app", "-f", "mount_path=/data")
	if err != nil || !called {
		t.Fatalf("called=%v err=%v", called, err)
	}
}
