package cli

import (
	"encoding/json"
	"testing"
)

func TestNormalizePath(t *testing.T) {
	cases := map[string]string{
		"/vpcs":                         "/vpcs",
		"vpcs":                          "/vpcs",
		"/v1/vpcs":                      "/vpcs",
		"https://api.tatnet.ru/v1/vpcs": "/vpcs",
		"/v1/projects/abc/vms/":         "/projects/abc/vms",
	}
	for in, want := range cases {
		if got := normalizePath(in); got != want {
			t.Errorf("normalizePath(%q) = %q, ожидалось %q", in, got, want)
		}
	}
}

// Поля тела должны уезжать своим типом: числовой параметр строкой сервер
// отвергает 422, и причина выглядит как ошибка пользователя.
func TestTypedValue(t *testing.T) {
	if v := typedValue("3"); v != int64(3) {
		t.Errorf("число разобрано как %#v", v)
	}
	if v := typedValue("true"); v != true {
		t.Errorf("булево разобрано как %#v", v)
	}
	if v := typedValue("null"); v != nil {
		t.Errorf("null разобран как %#v", v)
	}
	if v := typedValue("web-1"); v != "web-1" {
		t.Errorf("строка искажена: %#v", v)
	}
	if v, ok := typedValue(`["a","b"]`).([]any); !ok || len(v) != 2 {
		t.Errorf("массив не разобран: %#v", v)
	}
}

func TestBuildBodyFromFields(t *testing.T) {
	body, err := buildBody(nil, []string{"name=web", "count=2"}, []string{"raw=3"}, "")
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatal(err)
	}
	if m["name"] != "web" || m["count"].(float64) != 2 || m["raw"] != "3" {
		t.Fatalf("тело собрано неверно: %s", body)
	}
}

func TestBuildBodyRejectsMixedSources(t *testing.T) {
	if _, err := buildBody(nil, []string{"a=b"}, nil, "file.json"); err == nil {
		t.Error("--input вместе с -f принят")
	}
}

func TestUnknownPathErrorSuggests(t *testing.T) {
	err := unknownPathError("GET", "/vpc")
	if err == nil {
		t.Fatal("несуществующий путь принят")
	}
	if got := err.Error(); !contains(got, "/vpcs") {
		t.Errorf("отказ не подсказал похожий адрес:\n%s", got)
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (func() bool {
		for i := 0; i+len(needle) <= len(haystack); i++ {
			if haystack[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	})()
}
