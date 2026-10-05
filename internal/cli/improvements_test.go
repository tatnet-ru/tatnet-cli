package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/tatnet-ru/tatnet-cli/internal/output"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

const observeApp = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1"
const observeProject = "11111111-1111-1111-1111-111111111111"

func TestAPIQueryValidatedByPath(t *testing.T) {
	for _, path := range []string{"/apps?limit=7&offset=2", "/v1/apps?limit=7&offset=2", "https://api.tatnet.ru/v1/apps?limit=7&offset=2"} {
		t.Run(path, func(t *testing.T) {
			called := false
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				if r.URL.Path != "/apps" || r.URL.Query().Get("limit") != "7" || r.URL.Query().Get("offset") != "2" {
					t.Errorf("unexpected URL %s", r.URL)
				}
				writeJSON(w, page(nil, 2, 7))
			}))
			defer srv.Close()
			_, _, err := run(t, srv, "api", path)
			if err != nil || !called {
				t.Fatalf("called=%v err=%v", called, err)
			}
		})
	}
}

func TestAppFiltersAfterPaginationAndAllProjects(t *testing.T) {
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/apps" {
			requests++
			if r.URL.Query().Get("project_id") != "" {
				t.Error("all-projects inherited project")
			}
			offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
			size, _ := strconv.Atoi(r.URL.Query().Get("limit"))
			data := []any{}
			for i := offset; i < offset+size && i < 201; i++ {
				name := "other"
				if i == 200 {
					name = "wanted"
				}
				data = append(data, map[string]any{"id": observeApp, "project_id": observeProject, "name": name, "repo_full_name": "org/repo", "branch": "main"})
			}
			writeJSON(w, page(data, offset, size))
			return
		}
		if strings.HasSuffix(r.URL.Path, "/domains") {
			writeJSON(w, page([]any{map[string]any{"domain": "web.example.com"}}, 0, 100))
			return
		}
		t.Errorf("unexpected %s", r.URL)
		w.WriteHeader(404)
	}))
	defer srv.Close()
	out, _, err := run(t, srv, "app", "list", "--all-projects", "--name", "wanted", "--repo", "org/repo", "--branch", "main", "--domain", "WEB.EXAMPLE.COM.", "--limit", "1", "-o", "json")
	if err != nil {
		t.Fatal(err)
	}
	var apps []any
	if err = json.Unmarshal([]byte(out), &apps); err != nil || len(apps) != 1 || requests < 2 {
		t.Fatalf("out=%s requests=%d err=%v", out, requests, err)
	}
	_, _, err = run(t, srv, "app", "list", "--all-projects", "--project", observeProject)
	if err == nil {
		t.Fatal("conflicting scope flags accepted")
	}
}

func TestAppWaitRequiresLiveAndExactCommit(t *testing.T) {
	for _, state := range []string{"live", "rolling", "", "unverified", "failing", "never_booted", "stale_serving", "booted"} {
		t.Run(state, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case strings.HasSuffix(r.URL.Path, "/logs"):
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = w.Write([]byte("data: startup failed\nevent: done\n"))
				case strings.HasSuffix(r.URL.Path, "/builds"):
					writeJSON(w, page([]any{map[string]any{"id": "newer", "app_id": observeApp, "status": "success", "deploy_state": "live", "commit_sha": "other"}, map[string]any{"id": "target", "app_id": observeApp, "status": "success", "deploy_state": state, "commit_sha": "wanted", "boot_error": "cannot start"}}, 0, 100))
				default:
					t.Errorf("unexpected %s", r.URL)
					w.WriteHeader(404)
				}
			}))
			defer srv.Close()
			var out, errOut bytes.Buffer
			e := &Env{APIKey: "key", BaseURL: srv.URL, Timeout: time.Second, Printer: output.Printer{Out: &out, Err: &errOut, Format: output.JSON}}
			c, err := e.Client()
			if err != nil {
				t.Fatal(err)
			}
			cmd := NewRootCommand("test")
			cmd.SetOut(&out)
			cmd.SetErr(&errOut)
			cmd.SetContext(context.Background())
			b, err := e.waitLive(cmd, c, observeProject, observeApp, "", "wanted", 50*time.Millisecond, false)
			if state == "live" {
				if err != nil || output.Value(b, "id") != "target" {
					t.Fatalf("%v %v", b, err)
				}
			} else if err == nil {
				t.Fatalf("non-live state %q accepted", state)
			}
			if state == "failing" && !strings.Contains(errOut.String(), "startup failed") {
				t.Fatal("missing failure log")
			}
		})
	}
}

func TestNetworkingCommandsUseAccountPaths(t *testing.T) {
	cases := []struct {
		args               []string
		method, path, body string
	}{
		{[]string{"vpc", "list"}, "GET", "/vpcs", ""},
		{[]string{"vpc", "create", "--name", "private", "--cluster", "region", "--subnet", "10.20.0.0/24"}, "POST", "/vpcs", "subnet"},
		{[]string{"vpc", "get", observeProject}, "GET", "/vpcs/" + observeProject, ""},
		{[]string{"vpc", "delete", observeProject, "--yes"}, "DELETE", "/vpcs/" + observeProject, ""},
		{[]string{"vpc", "nat", "enable", observeProject, "--floating-ip", "fip"}, "POST", "/vpcs/" + observeProject + "/nat-gateway", "fip_id"},
		{[]string{"vpc", "nat", "disable", observeProject, "--yes"}, "DELETE", "/vpcs/" + observeProject + "/nat-gateway", ""},
		{[]string{"vpc", "reserved-ip", "create", observeProject, "--name", "reserved", "--address", "10.20.0.5", "--mac", "02:00:00:00:00:01"}, "POST", "/vpcs/" + observeProject + "/reserved-ips", "address"},
		{[]string{"vpc", "reserved-ip", "delete", observeProject, "reserved-id", "--yes"}, "DELETE", "/vpcs/" + observeProject + "/reserved-ips/reserved-id", ""},
		{[]string{"floating-ip", "list"}, "GET", "/floating-ips", ""},
		{[]string{"floating-ip", "create", "--cluster", "region", "--name", "public"}, "POST", "/floating-ips", "cluster_id"},
		{[]string{"floating-ip", "get", observeApp}, "GET", "/floating-ips/" + observeApp, ""},
		{[]string{"floating-ip", "update", observeApp, "--name", "public"}, "PATCH", "/floating-ips/" + observeApp, "name"},
		{[]string{"floating-ip", "attach", observeApp, "--interface", "interface-id"}, "POST", "/floating-ips/" + observeApp + "/attach", "vm_interface_id"},
		{[]string{"floating-ip", "detach", observeApp, "--yes"}, "POST", "/floating-ips/" + observeApp + "/detach", ""},
		{[]string{"floating-ip", "delete", observeApp, "--yes"}, "DELETE", "/floating-ips/" + observeApp, ""},
	}
	for _, tc := range cases {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			called := false
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				if r.Method != tc.method || r.URL.Path != tc.path {
					t.Errorf("got %s %s", r.Method, r.URL.Path)
				}
				if tc.body != "" {
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					if body[tc.body] == nil {
						t.Errorf("body %v", body)
					}
				}
				w.Header().Set("Content-Type", "application/json")
				if tc.method == "GET" && (tc.path == "/vpcs" || tc.path == "/floating-ips") {
					writeJSON(w, page(nil, 0, 100))
				} else if tc.method == "DELETE" {
					w.WriteHeader(204)
				} else {
					writeJSON(w, map[string]any{"id": observeProject, "name": "private", "cluster_id": "region", "subnet": "10.20.0.0/24", "address": "1.2.3.4", "status": "active"})
				}
			}))
			defer srv.Close()
			_, _, err := run(t, srv, tc.args...)
			if err != nil || !called {
				t.Fatalf("called=%v err=%v", called, err)
			}
		})
	}
}

func TestConfiguredProjectScopeIsVisibleAndCanBeOverridden(t *testing.T) {
	var query string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.Query().Get("project_id")
		writeJSON(w, page(nil, 0, 100))
	}))
	defer srv.Close()
	for _, all := range []bool{false, true} {
		t.Run(strconv.FormatBool(all), func(t *testing.T) {
			t.Setenv("TATNET_PROJECT", observeProject)
			t.Setenv("TATNET_CONFIG", t.TempDir()+"/config.yaml")
			t.Setenv("TATNET_API_KEY", "key")
			t.Setenv("TATNET_BASE_URL", srv.URL)
			var out, errOut bytes.Buffer
			root := NewRootCommand("test")
			root.SetOut(&out)
			root.SetErr(&errOut)
			args := []string{"app", "list"}
			if all {
				args = append(args, "--all-projects")
			}
			root.SetArgs(args)
			if err := root.Execute(); err != nil {
				t.Fatal(err)
			}
			if all {
				if query != "" || !strings.Contains(errOut.String(), "все доступные проекты") {
					t.Fatalf("query=%s stderr=%s", query, errOut.String())
				}
			} else if query != observeProject || !strings.Contains(errOut.String(), "$TATNET_PROJECT") {
				t.Fatalf("query=%s stderr=%s", query, errOut.String())
			}
		})
	}
}

func TestAppDeployWaitTracksReturnedBuild(t *testing.T) {
	deployed := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/apps/"+observeApp:
			writeJSON(w, map[string]any{"id": observeApp, "name": "web", "project_id": observeProject})
		case strings.HasSuffix(r.URL.Path, "/deploy"):
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["commit_sha"] != "wanted" {
				t.Error(body)
			}
			deployed = true
			writeJSON(w, map[string]any{"id": "target", "app_id": observeApp, "status": "queued"})
		case strings.HasSuffix(r.URL.Path, "/builds"):
			writeJSON(w, page([]any{map[string]any{"id": "newer", "status": "success", "deploy_state": "live"}, map[string]any{"id": "target", "status": "success", "deploy_state": "live"}}, 0, 100))
		default:
			t.Errorf("unexpected %s", r.URL)
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	out, _, err := run(t, srv, "app", "deploy", observeApp, "--commit", "wanted", "--wait", "-o", "json")
	if err != nil || !deployed || !strings.Contains(out, `"id": "target"`) {
		t.Fatalf("%s %v", out, err)
	}
}

func TestWaitPinsSelectedCommitBuild(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		builds := []any{map[string]any{"id": "target", "status": "success", "deploy_state": "rolling", "commit_sha": "wanted"}}
		if calls > 1 {
			builds = []any{map[string]any{"id": "newer", "status": "success", "deploy_state": "live", "commit_sha": "wanted"}, map[string]any{"id": "target", "status": "success", "deploy_state": "failing", "commit_sha": "wanted"}}
		}
		if strings.HasSuffix(r.URL.Path, "/logs") {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("event: done\n"))
			return
		}
		writeJSON(w, page(builds, 0, 100))
	}))
	defer srv.Close()
	e := &Env{APIKey: "key", BaseURL: srv.URL, Timeout: time.Second}
	c, err := e.Client()
	if err != nil {
		t.Fatal(err)
	}
	cmd := NewRootCommand("test")
	cmd.SetContext(context.Background())
	var logs bytes.Buffer
	cmd.SetErr(&logs)
	_, err = e.waitLive(cmd, c, observeProject, observeApp, "", "wanted", 3*time.Second, false)
	if err == nil || !strings.Contains(err.Error(), "target") {
		t.Fatalf("wait accepted another live build: %v", err)
	}
}
