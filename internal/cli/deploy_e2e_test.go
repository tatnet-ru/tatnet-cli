package cli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// deployAPI — подставной /v1 для деплоя из папки. Считает, сколько раз
// создавали приложение: повторная выкладка обязана попадать в то же самое.
type deployAPI struct {
	t            *testing.T
	appsCreated  int
	uploads      int
	lastArchive  []byte
	buildStatus  string
	buildErrText string
	domains      []any
	appExists    bool
	logCalls     int
	logLines     []string
}

const (
	testProjectID = "11111111-1111-1111-1111-111111111111"
	testAppID     = "22222222-2222-2222-2222-222222222222"
	testBuildID   = "33333333-3333-3333-3333-333333333333"
)

func (f *deployAPI) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		p := r.URL.Path
		switch {
		case p == "/projects" && r.Method == http.MethodGet:
			writeJSON(w, page([]any{map[string]any{"id": testProjectID, "name": "проект"}}, 0, 100))

		case p == "/projects/"+testProjectID+"/apps" && r.Method == http.MethodPost:
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["source_type"] != "upload" {
				f.t.Errorf("приложение создано с source_type=%v, ожидался upload", body["source_type"])
			}
			f.appsCreated++
			f.appExists = true
			w.WriteHeader(http.StatusCreated)
			writeJSON(w, map[string]any{
				"id": testAppID, "project_id": testProjectID, "name": body["name"],
				"status": "inactive", "source_type": "upload",
			})

		case p == "/projects/"+testProjectID+"/apps/"+testAppID && r.Method == http.MethodGet:
			if !f.appExists {
				w.WriteHeader(http.StatusNotFound)
				writeJSON(w, map[string]any{"detail": "App not found"})
				return
			}
			writeJSON(w, map[string]any{
				"id": testAppID, "project_id": testProjectID, "name": "folder-app",
				"status": "inactive", "source_type": "upload",
			})

		case p == "/projects/"+testProjectID+"/apps/"+testAppID+"/deployments" && r.Method == http.MethodPost:
			if ct := r.Header.Get("Content-Type"); ct != "application/gzip" {
				f.t.Errorf("Content-Type = %q, ожидался application/gzip", ct)
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				f.t.Fatalf("тело не прочиталось: %v", err)
			}
			f.lastArchive = body
			f.uploads++
			w.WriteHeader(http.StatusAccepted)
			writeJSON(w, map[string]any{
				"id": testBuildID, "app_id": testAppID,
				"site_id": testProjectID + "-" + testAppID, "status": "queued",
			})

		case strings.HasSuffix(p, "/builds") && r.Method == http.MethodGet:
			writeJSON(w, page([]any{map[string]any{
				"id": testBuildID, "app_id": testAppID,
				"status": f.buildStatus, "error": f.buildErrText,
			}}, 0, 20))

		case strings.HasSuffix(p, "/logs") && r.Method == http.MethodGet:
			// Первый запрос — живой поток, который оборвался на середине
			// (сборка закончилась быстрее, чем доехал её вывод). Второй —
			// полная история у завершённой сборки, как и отдаёт API.
			f.logCalls++
			w.Header().Set("Content-Type", "text/event-stream")
			lines := f.logLines
			if f.logCalls == 1 {
				lines = lines[:1]
			}
			for _, l := range lines {
				fmt.Fprintf(w, "data: %s\n\n", l)
			}
			fmt.Fprint(w, "event: done\ndata: {}\n\n")

		case strings.HasSuffix(p, "/domains") && r.Method == http.MethodGet:
			writeJSON(w, page(f.domains, 0, 100))

		default:
			f.t.Errorf("неожиданный запрос: %s %s", r.Method, p)
			w.WriteHeader(http.StatusNotFound)
			writeJSON(w, map[string]any{"detail": "not found"})
		}
	})
}

func newDeployAPI(t *testing.T) *deployAPI {
	return &deployAPI{
		t:           t,
		buildStatus: "success",
		logLines:    []string{"Starting build...", "Unpacking uploaded sources...", "Published 3 files"},
		domains: []any{
			map[string]any{"id": "d2", "app_id": testAppID, "domain": "своё.example", "status": "active", "is_default": false},
			map[string]any{"id": "d1", "app_id": testAppID, "domain": "folder-app-abc.tatnet.app", "status": "active", "is_default": true},
		},
	}
}

func projectDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for rel, body := range map[string]string{
		"index.html":    "<h1>из папки</h1>",
		"src/app.js":    "console.log(1)",
		".gitignore":    "dist/\n",
		"dist/stale.js": "старое",
		".env":          "SECRET=боевой",
	} {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func archiveNames(t *testing.T, gzData []byte) []string {
	t.Helper()
	zr, err := gzip.NewReader(bytes.NewReader(gzData))
	if err != nil {
		t.Fatalf("загружено не gzip: %v", err)
	}
	tr := tar.NewReader(zr)
	var names []string
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("архив битый: %v", err)
		}
		if hdr.Typeflag == tar.TypeReg {
			names = append(names, hdr.Name)
		}
	}
	sort.Strings(names)
	return names
}

// Контракт вывода: в stdout уходит ТОЛЬКО адрес. На него пишут скрипты
// (`url=$(tatnet deploy)`), и любая лишняя строка их ломает.
func TestE2EDeploy_StdoutIsOnlyTheURL(t *testing.T) {
	api := newDeployAPI(t)
	srv := httptest.NewServer(api.handler())
	defer srv.Close()
	dir := projectDir(t)

	out, errOut, err := run(t, srv, "deploy", dir, "-p", testProjectID)
	if err != nil {
		t.Fatalf("deploy: %v\n%s", err, errOut)
	}
	if out != "https://folder-app-abc.tatnet.app\n" {
		t.Errorf("stdout = %q, ожидался только адрес по умолчанию", out)
	}
	if !strings.Contains(errOut, "Упаковано") {
		t.Errorf("stderr не рассказал, что уехало:\n%s", errOut)
	}
}

// В архив едут исходники — и не едут ни .env, ни исключённое правилами.
func TestE2EDeploy_ArchiveContents(t *testing.T) {
	api := newDeployAPI(t)
	srv := httptest.NewServer(api.handler())
	defer srv.Close()
	dir := projectDir(t)

	_, errOut, err := run(t, srv, "deploy", dir, "-p", testProjectID)
	if err != nil {
		t.Fatalf("deploy: %v\n%s", err, errOut)
	}
	names := archiveNames(t, api.lastArchive)
	got := strings.Join(names, " ")
	for _, want := range []string{"index.html", "src/app.js"} {
		if !strings.Contains(got, want) {
			t.Errorf("в архиве нет %s: %v", want, names)
		}
	}
	for _, unwanted := range []string{".env", "dist/stale.js"} {
		for _, n := range names {
			if n == unwanted {
				t.Errorf("в архив уехало лишнее: %s", n)
			}
		}
	}
	if !strings.Contains(errOut, ".env") {
		t.Error("о пропущенном .env не сказано ни слова — человек узнает об этом по неработающему приложению")
	}
}

// Вторая выкладка той же папки обязана попасть в ТО ЖЕ приложение.
func TestE2EDeploy_SecondRunReusesTheApp(t *testing.T) {
	api := newDeployAPI(t)
	srv := httptest.NewServer(api.handler())
	defer srv.Close()
	dir := projectDir(t)

	if _, errOut, err := run(t, srv, "deploy", dir, "-p", testProjectID); err != nil {
		t.Fatalf("первая выкладка: %v\n%s", err, errOut)
	}
	if _, errOut, err := run(t, srv, "deploy", dir, "-p", testProjectID); err != nil {
		t.Fatalf("вторая выкладка: %v\n%s", err, errOut)
	}
	if api.appsCreated != 1 {
		t.Errorf("приложений создано %d, ожидалось одно — иначе каждая выкладка даёт новый адрес", api.appsCreated)
	}
	if api.uploads != 2 {
		t.Errorf("загрузок %d, ожидалось 2", api.uploads)
	}
	if _, err := os.Stat(filepath.Join(dir, ".tatnet", "project.json")); err != nil {
		t.Errorf("привязка не записана: %v", err)
	}
}

// Неудачная сборка — ненулевой код возврата и причина, а не адрес.
func TestE2EDeploy_FailedBuildIsAnError(t *testing.T) {
	api := newDeployAPI(t)
	api.buildStatus = "error"
	api.buildErrText = "build command failed: exit 1"
	srv := httptest.NewServer(api.handler())
	defer srv.Close()
	dir := projectDir(t)

	out, _, err := run(t, srv, "deploy", dir, "-p", testProjectID)
	if err == nil {
		t.Fatal("упавшая сборка завершилась успехом")
	}
	if !strings.Contains(err.Error(), "build command failed") {
		t.Errorf("причина отказа не показана: %v", err)
	}
	if strings.Contains(out, "https://") {
		t.Errorf("адрес напечатан при неудачной сборке: %q", out)
	}
}

// --no-create без привязки: отказ, а не молчаливое создание.
func TestE2EDeploy_NoCreateWithoutLink(t *testing.T) {
	api := newDeployAPI(t)
	srv := httptest.NewServer(api.handler())
	defer srv.Close()
	dir := projectDir(t)

	_, _, err := run(t, srv, "deploy", dir, "-p", testProjectID, "--no-create")
	if err == nil {
		t.Fatal("--no-create не помешал выкладке")
	}
	if api.appsCreated != 0 {
		t.Errorf("приложение всё-таки создано: %d", api.appsCreated)
	}
}

// Пустая папка: отказ ДО загрузки, с понятной причиной.
func TestE2EDeploy_EmptyFolder(t *testing.T) {
	api := newDeployAPI(t)
	srv := httptest.NewServer(api.handler())
	defer srv.Close()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "index.html"), []byte("x"), 0o644)

	_, _, err := run(t, srv, "deploy", dir, "-p", testProjectID)
	if err == nil {
		t.Fatal("пустая после правил папка выложилась")
	}
	if api.uploads != 0 {
		t.Error("пустой архив всё-таки ушёл на сервер")
	}
}

// Короткая сборка заканчивается раньше, чем доезжает её собственный вывод:
// живой поток закрывается по терминальному статусу. Замер на проде 16.09:
// клиент показал 3 строки из 16 и написал «Сборка прошла» — вывод выглядел
// полным и не был им. Хвост обязан дочитываться, и без повторов.
func TestE2EDeploy_LogTailIsReadAfterTheBuildEnds(t *testing.T) {
	api := newDeployAPI(t)
	srv := httptest.NewServer(api.handler())
	defer srv.Close()
	dir := projectDir(t)

	_, errOut, err := run(t, srv, "deploy", dir, "-p", testProjectID, "--logs")
	if err != nil {
		t.Fatalf("deploy: %v\n%s", err, errOut)
	}
	for _, line := range api.logLines {
		if strings.Count(errOut, line) != 1 {
			t.Errorf("строка лога %q встречается %d раз, ожидался ровно один",
				line, strings.Count(errOut, line))
		}
	}
	if api.logCalls != 2 {
		t.Errorf("обращений к логу %d, ожидалось 2 (живой поток и дочитывание хвоста)", api.logCalls)
	}
}
