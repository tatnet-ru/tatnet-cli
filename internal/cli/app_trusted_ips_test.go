package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestAppTrustedIPs(t *testing.T) {
	const appID = "33333333-3333-3333-3333-333333333333"
	const projectID = "11111111-1111-1111-1111-111111111111"
	for _, tt := range []struct {
		action, method string
		ips            []string
	}{
		{"get", http.MethodGet, nil},
		{"set", http.MethodPut, []string{"192.0.2.1", "198.51.100.0/24"}},
		{"clear", http.MethodPut, []string{}},
	} {
		t.Run(tt.action, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/v1/apps/" + appID:
					_, _ = w.Write([]byte(`{"id":"` + appID + `","project_id":"` + projectID + `"}`))
				case "/v1/projects/" + projectID + "/apps/" + appID + "/trusted-ips":
					if r.Method != tt.method {
						t.Errorf("method = %s, want %s", r.Method, tt.method)
					}
					if r.Method == http.MethodPut {
						var body struct {
							TrustedIPs []string `json:"trusted_ips"`
						}
						if err := json.NewDecoder(r.Body).Decode(&body); err != nil || !reflect.DeepEqual(body.TrustedIPs, tt.ips) {
							t.Errorf("addresses = %v, want %v (%v)", body.TrustedIPs, tt.ips, err)
						}
					}
					_, _ = w.Write([]byte(`{"trusted_ips":[]}`))
				default:
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer srv.Close()
			args := []string{"app", "trusted-ips", tt.action, appID}
			if tt.action == "set" {
				args = append(args, tt.ips...)
			}
			if _, _, err := run(t, srv, args...); err != nil {
				t.Fatal(err)
			}
		})
	}
}
