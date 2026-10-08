package cli

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAIUsesInferenceKeyAndPublicGateway(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/v1/images/jobs" || r.Header.Get("Authorization") != "Bearer tnai_live_test" {
			t.Errorf("%s auth=%s", r.URL.Path, r.Header.Get("Authorization"))
		}
		w.WriteHeader(202)
		w.Write([]byte(`{"id":"img_a"}`))
	}))
	defer server.Close()
	t.Setenv("TATNET_INFERENCE_API_KEY", "tnai_live_test")
	root := NewRootCommand("test")
	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetArgs([]string{"ai", "generate", "--inference-base-url", server.URL + "/v1", "--body", `{"model":"kie-grok-imagine","prompt":"a"}`})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || !strings.Contains(out.String(), "img_a") {
		t.Fatal(calls, out.String())
	}
}
func TestAIPaidSubmitDoesNotFollowRedirect(t *testing.T) {
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; http.Redirect(w, r, "/again", 307) }))
	defer s.Close()
	t.Setenv("TATNET_INFERENCE_API_KEY", "tnai_live_test")
	root := NewRootCommand("test")
	root.SetArgs([]string{"ai", "generate", "--inference-base-url", s.URL, "--body", "{}"})
	if err := root.Execute(); err == nil {
		t.Fatal("redirect accepted")
	}
	if calls != 1 {
		t.Fatal(calls)
	}
}

func TestAIChatCatalogIsPublic(t *testing.T) {
	for _, kind := range []string{"chat", "text"} {
		t.Run(kind, func(t *testing.T) {
			t.Setenv("TATNET_INFERENCE_API_KEY", "")
			calls := 0
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.URL.Path != "/v1/models" || r.Header.Get("Authorization") != "" || r.Method != "GET" {
					t.Errorf("wrong request: %s %s", r.Method, r.URL)
				}
				w.Write([]byte(`{"data":[{"id":"glm-5.2"}]}`))
			}))
			defer s.Close()
			root := NewRootCommand("test")
			root.SetOut(&bytes.Buffer{})
			root.SetArgs([]string{"ai", "models", "--kind", kind, "--inference-base-url", s.URL + "/v1"})
			if err := root.Execute(); err != nil {
				t.Fatal(err)
			}
			if calls != 1 {
				t.Fatal(calls)
			}
		})
	}
}
