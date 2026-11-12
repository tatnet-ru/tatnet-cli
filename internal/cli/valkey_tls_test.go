package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestValkeyTLSFlagReachesAPI(t *testing.T) {
	for _, create := range []bool{false, true} {
		t.Run(map[bool]string{true: "create", false: "params"}[create], func(t *testing.T) {
			var got map[string]any
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer tn_live_тест" {
					t.Error("missing caller credentials")
				}
				if r.Method != "GET" {
					if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
						t.Error(err)
					}
				}
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"id":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1","project_id":"11111111-1111-1111-1111-111111111111","tls_mode":"preferred"}`))
			}))
			defer srv.Close()
			args := []string{"--project", "11111111-1111-1111-1111-111111111111", "--output", "json", "valkey"}
			if create {
				args = append(args, "create", "--name", "cache", "--plan", "p")
			} else {
				args = append(args, "params", "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1")
			}
			args = append(args, "--tls-mode", "preferred")
			out, _, err := run(t, srv, args...)
			if err != nil {
				t.Fatal(err)
			}
			if got["tls_mode"] != "preferred" || !strings.Contains(out, `"tls_mode"`) {
				t.Fatalf("body=%v output=%s", got, out)
			}
		})
	}
}
