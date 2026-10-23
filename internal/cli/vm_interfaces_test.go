package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestVMInterfaceCommandsKeepOwnerProjectAndPayload(t *testing.T) {
	const project = "11111111-1111-1111-1111-111111111111"
	const vm = "33333333-3333-3333-3333-333333333333"
	for _, method := range []string{"POST", "DELETE"} {
		t.Run(method, func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "GET" {
					writeJSON(w, map[string]any{"id": vm, "project_id": project, "name": "VM", "hostname": "vm", "status": "stopped", "vcpu": 1, "mem": 1024, "disk_size": 10})
					return
				}
				calls++
				expected := "/projects/" + project + "/vms/" + vm + "/interfaces"
				if method == "DELETE" {
					expected += "/nic"
				}
				if r.Method != method || r.URL.Path != expected || r.Header.Get("Authorization") != "Bearer tn_live_тест" {
					t.Fatalf("wrong request: %s %s", r.Method, r.URL.Path)
				}
				if method == "POST" {
					var body map[string]string
					json.NewDecoder(r.Body).Decode(&body)
					if body["vpc_id"] != "vpc" {
						t.Fatalf("wrong VPC: %v", body)
					}
				}
				writeJSON(w, map[string]any{"status": "success", "interface_id": "nic"})
			}))
			defer srv.Close()
			args := []string{"vm", "interface", "attach", vm, "--vpc", "vpc", "-p", project}
			if method == "DELETE" {
				args = []string{"vm", "interface", "detach", vm, "nic", "-p", project}
			}
			_, _, err := run(t, srv, args...)
			if err != nil {
				t.Fatal(err)
			}
			if calls != 1 {
				t.Fatalf("sent %d mutations", calls)
			}
		})
	}
}
