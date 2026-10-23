package cli

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDomainPolicyCommandsMatchRefreshedContract(t *testing.T) {
	for _, action := range []string{"get", "set", "restore"} {
		t.Run(action, func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/apps/33333333-3333-3333-3333-333333333333" {
					writeJSON(w, map[string]string{"id": "33333333-3333-3333-3333-333333333333", "project_id": "11111111-1111-1111-1111-111111111111", "name": "web", "status": "active"})
					return
				}
				calls++
				want := "/projects/11111111-1111-1111-1111-111111111111/apps/33333333-3333-3333-3333-333333333333/domain-policy"
				if action == "restore" {
					want += "/restore"
				}
				if r.URL.Path != want {
					t.Fatalf("wrong path %s", r.URL.Path)
				}
				method := map[string]string{"get": "GET", "set": "PATCH", "restore": "POST"}[action]
				if r.Method != method {
					t.Fatalf("wrong method %s", r.Method)
				}
				writeJSON(w, map[string]string{"mode": "serve"})
			}))
			defer srv.Close()
			args := []string{"app", "domain-policy", action, "33333333-3333-3333-3333-333333333333"}
			if action == "set" {
				args = append(args, "--mode", "serve")
			}
			if action == "restore" {
				args = append(args, "--domain", "web.tatnet.app")
			}
			if _, _, err := run(t, srv, args...); err != nil {
				t.Fatal(err)
			}
			if calls != 1 {
				t.Fatalf("sent %d requests", calls)
			}
		})
	}
}
