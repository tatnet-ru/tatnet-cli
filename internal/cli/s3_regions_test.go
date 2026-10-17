package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestS3RegionCommands(t *testing.T) {
	for _, test := range []struct {
		name         string
		args         []string
		path, method string
	}{
		{"create", []string{"s3", "bucket", "create", "photos", "--region", "spb-1"}, "/object-storage/buckets", "POST"},
		{"connection", []string{"s3", "connection-info", "--region", "spb-1"}, "/object-storage/connection-info", "GET"},
		{"regions", []string{"s3", "regions"}, "/object-storage/regions", "GET"},
	} {
		t.Run(test.name, func(t *testing.T) {
			called := false
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				if r.URL.Path != test.path || r.Method != test.method {
					t.Errorf("%s %s", r.Method, r.URL)
				}
				if r.Header.Get("Authorization") == "" {
					t.Error("missing auth")
				}
				w.Header().Set("Content-Type", "application/json")
				switch test.name {
				case "create":
					var body map[string]any
					json.NewDecoder(r.Body).Decode(&body)
					if body["region"] != "spb-1" || body["name"] != "photos" {
						t.Errorf("body=%v", body)
					}
					w.WriteHeader(201)
					w.Write([]byte(`{"id":"one","name":"photos","region":"spb-1","status":"active"}`))
				case "connection":
					if r.URL.Query().Get("region") != "spb-1" {
						t.Errorf("query=%s", r.URL.RawQuery)
					}
					w.Write([]byte(`{"region":"spb-1","endpoint":"https://spb-1.s3.example","path_style_example":"x","virtual_hosted_example":"y"}`))
				default:
					w.Write([]byte(`{"regions":[{"region":"spb-1","enabled":true,"endpoint":"https://spb-1.s3.example"}]}`))
				}
			}))
			defer srv.Close()
			if _, _, err := run(t, srv, test.args...); err != nil {
				t.Fatal(err)
			}
			if !called {
				t.Fatal("API not called")
			}
		})
	}
}
