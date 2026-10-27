package cli

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNatDisableRetentionQuery(t *testing.T) {
	for _, retain := range []bool{false, true} {
		t.Run(map[bool]string{false: "release", true: "retain"}[retain], func(t *testing.T) {
			called := false
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				if r.Method != "DELETE" || r.URL.Path != "/vpcs/"+observeProject+"/nat-gateway" {
					t.Fatalf("unexpected request %s %s", r.Method, r.URL)
				}
				if got := r.URL.Query().Get("expected_fip_id"); got != observeApp {
					t.Errorf("expected_fip_id=%q, want %q", got, observeApp)
				}
				want := ""
				if retain {
					want = "true"
				}
				if got := r.URL.Query().Get("retain_ip"); got != want {
					t.Errorf("retain_ip=%q, want %q", got, want)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(202)
				writeJSON(w, map[string]any{"enabled": false, "status": "detaching", "fip_id": observeApp})
			}))
			defer srv.Close()
			args := []string{"vpc", "nat", "disable", observeProject, "--yes", "--expected-floating-ip", observeApp}
			if retain {
				args = append(args, "--retain-ip")
			}
			_, _, err := run(t, srv, args...)
			if err != nil {
				t.Fatal(err)
			}
			if !called {
				t.Fatal("API was not called")
			}
		})
	}
}

func TestNatDisableReturnsAllocationConflict(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		writeJSON(w, map[string]any{"error": map[string]any{"code": "nat_gateway_changed", "message": "The gateway has changed"}})
	}))
	defer srv.Close()
	_, _, err := run(t, srv, "vpc", "nat", "disable", observeProject, "--yes", "--retain-ip", "--expected-floating-ip", observeApp)
	if err == nil {
		t.Fatal("API conflict was ignored")
	}
}
