package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// fakeAPI — подставной сервер /v1. Проверяет заголовок авторизации и
// отдаёт страничные конверты того же вида, что и настоящий API.
type fakeAPI struct {
	t        *testing.T
	requests []string
	vms      int
}

func (f *fakeAPI) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer tn_live_тест" {
			f.t.Errorf("запрос без ключа или с чужим: %q", got)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		f.requests = append(f.requests, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery)
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.URL.Path == "/account":
			writeJSON(w, map[string]any{"account_id": "acc-1", "key_id": "key-1",
				"policy": []any{map[string]any{"effect": "allow", "actions": []any{"*"}, "resources": []any{"*"}}}})
		case r.URL.Path == "/projects":
			writeJSON(w, page([]any{
				map[string]any{"id": "11111111-1111-1111-1111-111111111111", "name": "прод"},
				map[string]any{"id": "22222222-2222-2222-2222-222222222222", "name": "тест"},
			}, 0, 100))
		case strings.HasSuffix(r.URL.Path, "/vms") && r.Method == http.MethodGet:
			offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
			limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
			var data []any
			for i := offset; i < offset+limit && i < f.vms; i++ {
				data = append(data, map[string]any{
					"id": fmt.Sprintf("vm-%03d", i), "name": fmt.Sprintf("web-%d", i),
					"hostname": fmt.Sprintf("web-%d", i), "status": "running",
					"vcpu": 2, "mem": 2048, "disk_size": 20,
					"ipv4_addresses": []any{"10.0.0." + strconv.Itoa(i%250)},
				})
			}
			writeJSON(w, page(data, offset, limit))
		case strings.HasSuffix(r.URL.Path, "/stop"):
			writeJSON(w, map[string]any{"status": "stopping", "message": "принято"})
		case r.URL.Path == "/vpcs":
			writeJSON(w, page([]any{map[string]any{"id": "vpc-1", "name": "сеть"}}, 0, 100))
		case r.URL.Path == "/ssh-keys" && r.Method == http.MethodPost:
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			writeJSON(w, map[string]any{"id": "key-9", "name": body["name"], "public_key": body["public_key"]})
		default:
			w.WriteHeader(http.StatusNotFound)
			writeJSON(w, map[string]any{"detail": "нет такого адреса: " + r.URL.Path})
		}
	})
}

func writeJSON(w http.ResponseWriter, v any) { _ = json.NewEncoder(w).Encode(v) }

func page(data []any, offset, limit int) map[string]any {
	if data == nil {
		data = []any{}
	}
	return map[string]any{"count": len(data), "data": data, "limit": limit, "offset": offset}
}

// run выполняет команду CLI против подставного сервера.
func run(t *testing.T, srv *httptest.Server, args ...string) (string, string, error) {
	t.Helper()
	t.Setenv("TATNET_CONFIG", filepath.Join(t.TempDir(), "config.yaml"))
	t.Setenv("TATNET_API_KEY", "tn_live_тест")
	t.Setenv("TATNET_BASE_URL", srv.URL)
	t.Setenv("TATNET_PROJECT", "")
	t.Setenv("TATNET_PROFILE", "")
	t.Setenv("TATNET_OUTPUT", "")

	var out, errOut bytes.Buffer
	root := NewRootCommand("test")
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), errOut.String(), err
}

func TestE2EAccount(t *testing.T) {
	api := &fakeAPI{t: t}
	srv := httptest.NewServer(api.handler())
	defer srv.Close()

	out, _, err := run(t, srv, "account")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "acc-1") {
		t.Fatalf("аккаунт не выведен: %q", out)
	}
}

// Проект указан именем: CLI обязан сам превратить его в идентификатор.
func TestE2EProjectResolvedByName(t *testing.T) {
	api := &fakeAPI{t: t, vms: 3}
	srv := httptest.NewServer(api.handler())
	defer srv.Close()

	out, _, err := run(t, srv, "vm", "list", "-p", "прод")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "web-0") {
		t.Fatalf("список ВМ пуст: %q", out)
	}
	var sawProjects, sawVMs bool
	for _, r := range api.requests {
		if strings.HasPrefix(r, "GET /projects?") {
			sawProjects = true
		}
		if strings.Contains(r, "/projects/11111111-1111-1111-1111-111111111111/vms") {
			sawVMs = true
		}
	}
	if !sawProjects || !sawVMs {
		t.Fatalf("имя проекта не разрешено в идентификатор: %v", api.requests)
	}
}

// Список длиннее страницы должен вычитываться целиком.
func TestE2EListReadsEveryPage(t *testing.T) {
	api := &fakeAPI{t: t, vms: 230}
	srv := httptest.NewServer(api.handler())
	defer srv.Close()

	out, _, err := run(t, srv, "vm", "list", "-p", "прод", "-o", "json")
	if err != nil {
		t.Fatal(err)
	}
	var items []any
	if err := json.Unmarshal([]byte(out), &items); err != nil {
		t.Fatalf("вывод не разбирается: %v", err)
	}
	if len(items) != 230 {
		t.Fatalf("вычитано %d ВМ из 230 — список оборван на странице", len(items))
	}
}

// Разрушающее действие без терминала обязано требовать --yes, а не
// соглашаться молча.
func TestE2EDestructiveNeedsConfirmation(t *testing.T) {
	api := &fakeAPI{t: t, vms: 1}
	srv := httptest.NewServer(api.handler())
	defer srv.Close()

	_, _, err := run(t, srv, "vm", "delete", "web-0", "-p", "прод")
	if err == nil {
		t.Fatal("удаление прошло без подтверждения")
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("отказ не объяснил, чего не хватает: %v", err)
	}
	for _, r := range api.requests {
		if strings.HasPrefix(r, "DELETE ") {
			t.Fatalf("несмотря на отказ, запрос ушёл: %s", r)
		}
	}
}

func TestE2EPowerAction(t *testing.T) {
	api := &fakeAPI{t: t, vms: 1}
	srv := httptest.NewServer(api.handler())
	defer srv.Close()

	out, _, err := run(t, srv, "vm", "stop", "web-0", "-p", "прод")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "stopping") {
		t.Fatalf("результат действия не выведен: %q", out)
	}
}

// Отказ API должен доходить до пользователя причиной, а не кодом.
func TestE2EAPIErrorReachesUser(t *testing.T) {
	api := &fakeAPI{t: t}
	srv := httptest.NewServer(api.handler())
	defer srv.Close()

	_, _, err := run(t, srv, "dns", "zone", "list")
	if err == nil {
		t.Fatal("отказ сервера принят за успех")
	}
	if !strings.Contains(err.Error(), "нет такого адреса") {
		t.Fatalf("причина отказа потеряна: %v", err)
	}
}

// Escape hatch обязан доставать то, для чего своей команды нет.
func TestE2EAPIEscapeHatch(t *testing.T) {
	api := &fakeAPI{t: t}
	srv := httptest.NewServer(api.handler())
	defer srv.Close()

	out, _, err := run(t, srv, "api", "/vpcs")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "сеть") {
		t.Fatalf("ответ не выведен: %q", out)
	}
}

// Несуществующий адрес не должен уезжать в сеть: он отсекается контрактом.
func TestE2EAPIRejectsUnknownPathBeforeSending(t *testing.T) {
	api := &fakeAPI{t: t}
	srv := httptest.NewServer(api.handler())
	defer srv.Close()

	_, _, err := run(t, srv, "api", "/vpc")
	if err == nil {
		t.Fatal("несуществующий адрес принят")
	}
	if len(api.requests) != 0 {
		t.Fatalf("запрос ушёл, несмотря на отказ: %v", api.requests)
	}
	if !strings.Contains(err.Error(), "/vpcs") {
		t.Errorf("отказ не подсказал верный адрес: %v", err)
	}
}

// Тело из -f должно уезжать своими типами и доходить до сервера.
func TestE2EAPIPostsTypedBody(t *testing.T) {
	api := &fakeAPI{t: t}
	srv := httptest.NewServer(api.handler())
	defer srv.Close()

	out, _, err := run(t, srv, "api", "/ssh-keys", "-X", "POST",
		"-f", "name=ноутбук", "-f", "public_key=ssh-ed25519 AAAA")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "ноутбук") {
		t.Fatalf("ответ не выведен: %q", out)
	}
}

// Без ключа команда обязана объяснить, как его задать, а не отдать 401.
func TestE2ENoKeyExplainsItself(t *testing.T) {
	api := &fakeAPI{t: t}
	srv := httptest.NewServer(api.handler())
	defer srv.Close()

	t.Setenv("TATNET_CONFIG", filepath.Join(t.TempDir(), "config.yaml"))
	t.Setenv("TATNET_API_KEY", "")
	t.Setenv("TATNET_BASE_URL", srv.URL)

	root := NewRootCommand("test")
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"account"})
	err := root.Execute()
	if err == nil {
		t.Fatal("без ключа команда отработала")
	}
	if !strings.Contains(err.Error(), "tatnet auth login") {
		t.Fatalf("отказ не объяснил, что делать: %v", err)
	}
}

// Проект не задан нигде — команда обязана сказать это словами.
func TestE2EMissingProjectIsExplained(t *testing.T) {
	api := &fakeAPI{t: t}
	srv := httptest.NewServer(api.handler())
	defer srv.Close()

	_, _, err := run(t, srv, "vm", "list")
	if err == nil {
		t.Fatal("команда без проекта отработала")
	}
	if !strings.Contains(err.Error(), "--project") {
		t.Fatalf("отказ не назвал недостающее: %v", err)
	}
}
