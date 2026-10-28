package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAPICreatesLoadBalancerWithReservedIP(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.Method != "POST" || r.URL.Path != "/projects/"+observeProject+"/load-balancers" {
			t.Fatalf("unexpected %s %s", r.Method, r.URL)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["floating_ip_id"] != "reserved" || body["vpc_id"] != "vpc" {
			t.Fatalf("wrong body %v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		writeJSON(w, map[string]any{"id": "lb", "floating_ip_id": "reserved"})
	}))
	defer srv.Close()
	if _, _, err := run(t, srv, "api", "/projects/"+observeProject+"/load-balancers", "-X", "POST", "-f", "name=web", "-f", "vpc_id=vpc", "-f", "floating_ip_id=reserved"); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("API not called")
	}
}

func TestAPIRetainsExactCurrentLoadBalancerIP(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusConflict} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" || r.URL.Path != "/projects/"+observeProject+"/load-balancers/lb/retain-ip" {
					t.Fatalf("unexpected %s %s", r.Method, r.URL)
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				if len(body) != 1 || body["expected_vip"] != "198.51.100.42" {
					t.Fatalf("wrong body %v", body)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				writeJSON(w, map[string]any{"id": "lb", "vip": "198.51.100.42", "floating_ip_id": "allocation"})
			}))
			defer srv.Close()
			out, _, err := run(t, srv, "api", "/projects/"+observeProject+"/load-balancers/lb/retain-ip", "-f", "expected_vip=198.51.100.42")
			if status == http.StatusConflict {
				if err == nil || !strings.Contains(err.Error(), "409") {
					t.Fatalf("409 hidden: %v", err)
				}
				return
			}
			if err != nil || !strings.Contains(out, "allocation") {
				t.Fatalf("result %q %v", out, err)
			}
		})
	}
}
