package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
