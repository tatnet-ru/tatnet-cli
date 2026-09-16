package deploy

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func write(t *testing.T, dir, rel, body string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func packed(t *testing.T, dir string) ([]string, Stats) {
	t.Helper()
	var buf bytes.Buffer
	st, err := Pack(dir, &buf, 0)
	if err != nil {
		t.Fatalf("Pack: %v", err)
	}
	gz, err := gzip.NewReader(&buf)
	if err != nil {
		t.Fatalf("архив не gzip: %v", err)
	}
	tr := tar.NewReader(gz)
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
		io.Copy(io.Discard, tr)
	}
	sort.Strings(names)
	return names, st
}

func TestPack_TakesTheTree(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "index.html", "<h1>hi</h1>")
	write(t, dir, "src/app.js", "1")
	write(t, dir, "public/logo.svg", "<svg/>")

	names, st := packed(t, dir)
	want := []string{"index.html", "public/logo.svg", "src/app.js"}
	if len(names) != len(want) {
		t.Fatalf("в архиве %v, ожидалось %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Errorf("[%d] = %q, want %q", i, names[i], want[i])
		}
	}
	if st.Files != 3 {
		t.Errorf("Files = %d, want 3", st.Files)
	}
}

// Тяжёлое и служебное не уезжает никогда, даже без файла правил.
func TestPack_AlwaysExcluded(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "index.html", "x")
	write(t, dir, ".git/config", "[core]")
	write(t, dir, "node_modules/left-pad/index.js", "module.exports=1")
	write(t, dir, ".tatnet/project.json", "{}")

	names, _ := packed(t, dir)
	for _, n := range names {
		if n != "index.html" {
			t.Errorf("в архив попало лишнее: %s", n)
		}
	}
}

// Ключи из папки не уезжают на прод — и об этом говорится вслух.
func TestPack_LeavesEnvFilesBehind(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "index.html", "x")
	write(t, dir, ".env", "TOKEN=боевой")
	write(t, dir, ".env.local", "TOKEN=тоже")
	write(t, dir, ".env.example", "TOKEN=")

	names, st := packed(t, dir)
	for _, n := range names {
		if n == ".env" || n == ".env.local" {
			t.Fatalf("файл с ключами уехал на сервер: %s", n)
		}
	}
	found := false
	for _, n := range names {
		if n == ".env.example" {
			found = true
		}
	}
	if !found {
		t.Error(".env.example — шаблон без значений, он должен остаться в архиве")
	}
	if len(st.SecretsHit) != 2 {
		t.Errorf("о пропущенных файлах окружения не сообщено: %v", st.SecretsHit)
	}
}

func TestPack_HonoursIgnoreFile(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, ".gitignore", "dist/\n*.log\n/only-root.txt\n!keep.log\n")
	write(t, dir, "index.html", "x")
	write(t, dir, "dist/bundle.js", "x")
	write(t, dir, "debug.log", "x")
	write(t, dir, "keep.log", "x")
	write(t, dir, "only-root.txt", "x")
	write(t, dir, "nested/only-root.txt", "x")

	names, st := packed(t, dir)
	got := map[string]bool{}
	for _, n := range names {
		got[n] = true
	}
	if got["dist/bundle.js"] {
		t.Error("каталог из правил попал в архив")
	}
	if got["debug.log"] {
		t.Error("*.log попал в архив")
	}
	if !got["keep.log"] {
		t.Error("отрицание «!» не вернуло файл")
	}
	if got["only-root.txt"] {
		t.Error("правило с ведущей косой чертой не сработало")
	}
	if !got["nested/only-root.txt"] {
		t.Error("правило с ведущей косой чертой не должно ловить вложенный файл")
	}
	if st.IgnoreFile != ".gitignore" {
		t.Errorf("IgnoreFile = %q", st.IgnoreFile)
	}
}

// `.tatnetignore` ЗАМЕНЯЕТ `.gitignore`, а не складывается с ним: иначе
// человек, написавший свой файл, всё равно не знал бы, что уедет.
func TestPack_TatnetIgnoreWins(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, ".gitignore", "secret.txt\n")
	write(t, dir, ".tatnetignore", "other.txt\n")
	write(t, dir, "secret.txt", "x")
	write(t, dir, "other.txt", "x")

	names, st := packed(t, dir)
	got := map[string]bool{}
	for _, n := range names {
		got[n] = true
	}
	if st.IgnoreFile != ".tatnetignore" {
		t.Fatalf("IgnoreFile = %q", st.IgnoreFile)
	}
	if got["other.txt"] {
		t.Error(".tatnetignore не применился")
	}
	if !got["secret.txt"] {
		t.Error(".gitignore не должен действовать рядом с .tatnetignore")
	}
}

func TestPack_EmptyFolderIsAnError(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, ".gitignore", "*\n")
	write(t, dir, "index.html", "x")
	var buf bytes.Buffer
	if _, err := Pack(dir, &buf, 0); err != ErrEmpty {
		t.Fatalf("ожидали ErrEmpty, получили %v", err)
	}
}

func TestPack_SizeLimit(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "big.bin", string(make([]byte, 2048)))
	var buf bytes.Buffer
	_, err := Pack(dir, &buf, 1024)
	if err == nil {
		t.Fatal("предел размера не сработал")
	}
}

func TestIgnore_DoubleStar(t *testing.T) {
	ig := &Ignore{}
	ig.add("**/tmp/")
	ig.add("docs/**/draft.md")
	cases := []struct {
		path  string
		isDir bool
		want  bool
	}{
		{"tmp", true, true},
		{"a/b/tmp", true, true},
		{"docs/x/y/draft.md", false, true},
		{"docs/draft.md", false, true},
		{"src/draft.md", false, false},
	}
	for _, c := range cases {
		if got := ig.Match(c.path, c.isDir); got != c.want {
			t.Errorf("Match(%q, dir=%v) = %v, want %v", c.path, c.isDir, got, c.want)
		}
	}
}
