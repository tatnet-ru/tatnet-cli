package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestSupabaseSQLRequiresExplicitConfirmationAndNeverRetries(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Header.Get("Authorization") != "Bearer tn_live_test" {
			t.Error("missing API key")
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["sql"] != "SELECT 1" {
			t.Error("wrong SQL body", body)
		}
		w.WriteHeader(503)
		w.Write([]byte(`{"error":{"message":"unknown outcome"}}`))
	}))
	defer srv.Close()
	run := func(yes bool) error {
		root := NewRootCommand("test")
		root.SetOut(&bytes.Buffer{})
		root.SetErr(&bytes.Buffer{})
		args := []string{"--api-key", "tn_live_test", "--base-url", srv.URL, "supabase", "sql", "1419b1ae-0aa4-436a-9769-c8fb13d9d770", "--sql", "SELECT 1"}
		if yes {
			args = append(args, "--yes")
		}
		root.SetArgs(args)
		return root.Execute()
	}
	if err := run(false); err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatal("SQL did not require confirmation", err)
	}
	if requests.Load() != 0 {
		t.Fatal("unconfirmed SQL was sent")
	}
	if err := run(true); err == nil {
		t.Fatal("timeout reported success")
	}
	if requests.Load() != 1 {
		t.Fatal("SQL was retried", requests.Load())
	}
}
