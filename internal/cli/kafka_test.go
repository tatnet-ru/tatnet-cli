package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestKafkaCreateUsesProjectAndPreservesHAAndVPC(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/projects/11111111-1111-1111-1111-111111111111/kafka-clusters" || r.Method != "POST" {
			t.Errorf("request %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") == "" {
			t.Error("missing auth")
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		w.WriteHeader(201)
		writeJSON(w, map[string]any{"id": "k", "password": "one-time"})
	}))
	defer srv.Close()
	out, _, err := run(t, srv, "--project", "11111111-1111-1111-1111-111111111111", "kafka", "create", "--name", "events", "--region", "r", "--plan", "b", "--controller-plan", "c", "--topology", "ha", "--vpc", "v", "--allow", "192.0.2.0/24")
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{"topology": "ha", "controller_plan_id": "c", "vpc_id": "v", "cluster_id": "r", "name": "events"} {
		if body[key] != want {
			t.Errorf("%s=%v", key, body[key])
		}
	}
	if !strings.Contains(out, "one-time") {
		t.Fatal("initial password omitted")
	}
}

func TestKafkaResumeUsesAuthenticatedProjectRouteAndPreservesBillingError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/projects/11111111-1111-1111-1111-111111111111/kafka-clusters/k/resume" || r.Header.Get("Authorization") == "" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(402)
		writeJSON(w, map[string]any{"detail": map[string]any{"code": "insufficient_balance", "shortfall": 12}})
	}))
	defer srv.Close()
	_, _, err := run(t, srv, "--project", "11111111-1111-1111-1111-111111111111", "kafka", "resume", "k")
	if err == nil || !strings.Contains(err.Error(), "insufficient_balance") {
		t.Fatalf("billing failure lost: %v", err)
	}
}

func TestRRsetDeleteRequiresRevisionAndSendsItToAPI(t *testing.T) {
	revision := strings.Repeat("a", 64)
	called := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		if r.Method != "DELETE" || r.URL.Path != "/dns/zones/z/rrsets/s" || r.URL.Query().Get("expected_revision") != revision {
			t.Errorf("request %s %s", r.Method, r.URL.String())
		}
		w.WriteHeader(204)
	}))
	defer srv.Close()
	_, _, err := run(t, srv, "dns", "rrset", "delete", "z", "s")
	if err == nil || called != 0 {
		t.Fatal("delete without revision reached the API")
	}
	_, _, err = run(t, srv, "dns", "rrset", "delete", "z", "s", "--revision", revision)
	if err != nil || called != 1 {
		t.Fatalf("delete failed: %v calls=%d", err, called)
	}
}

func TestKafkaMetricsAndPasswordUseScopedAPI(t *testing.T) {
	for _, tc := range []struct {
		args           []string
		method, suffix string
	}{
		{[]string{"reset-password", "k"}, "POST", "/k/password"},
		{[]string{"metrics", "k", "n", "--metric", "memory", "--range", "6h"}, "GET", "/k/nodes/n/metrics"},
	} {
		t.Run(tc.suffix, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != tc.method || !strings.HasSuffix(r.URL.Path, tc.suffix) || r.Header.Get("Authorization") == "" {
					t.Errorf("unexpected request %s %s", r.Method, r.URL)
				}
				if tc.method == "GET" && (r.URL.Query().Get("metric") != "memory" || r.URL.Query().Get("range") != "6h") {
					t.Error("metric selection lost")
				}
				writeJSON(w, map[string]any{"password": "one-time", "applied": false})
			}))
			defer srv.Close()
			args := append([]string{"--project", "11111111-1111-1111-1111-111111111111", "kafka"}, tc.args...)
			if _, _, err := run(t, srv, args...); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestKafkaHealthHistoryAndConfigPreserveScopeAndUnknown(t *testing.T) {
	for _, tc := range []struct {
		args           []string
		method, suffix string
	}{
		{[]string{"health", "k"}, "GET", "/k/monitoring"},
		{[]string{"history", "k", "--metric", "consumer_lag", "--group", "a&b/λ", "--range", "6h"}, "GET", "/k/monitoring/metrics"},
		{[]string{"monitoring-config", "k", "--disable-lag"}, "PUT", "/k/monitoring"},
	} {
		t.Run(tc.args[0], func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != tc.method || r.URL.Path != "/projects/11111111-1111-1111-1111-111111111111/kafka-clusters"+tc.suffix || r.Header.Get("Authorization") == "" {
					t.Errorf("unexpected request %s %s", r.Method, r.URL)
				}
				if tc.args[0] == "history" && (r.URL.Query().Get("group") != "a&b/λ" || r.URL.Query().Get("range") != "6h") {
					t.Error("group/range changed")
				}
				if r.Method == "PUT" {
					var b map[string]any
					if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
						t.Fatal(err)
					}
					if b["lag_threshold"] != nil || b["lag_duration_seconds"] != float64(300) {
						t.Errorf("wrong config %v", b)
					}
				}
				writeJSON(w, map[string]any{"lag": nil, "status": "partial"})
			}))
			defer srv.Close()
			out, _, err := run(t, srv, append([]string{"--project", "11111111-1111-1111-1111-111111111111", "kafka"}, tc.args...)...)
			if err != nil || !strings.Contains(out, "null") {
				t.Fatalf("unknown lag lost: %s %v", out, err)
			}
		})
	}
}
