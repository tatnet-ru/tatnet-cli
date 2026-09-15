package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func withConfig(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	t.Setenv("TATNET_CONFIG", path)
	return path
}

// Отсутствие конфига — не ошибка: CLI, настроенный переменными окружения,
// файла может не иметь вовсе.
func TestLoadMissingFileIsEmpty(t *testing.T) {
	withConfig(t)
	f, warn, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if warn != "" {
		t.Errorf("на отсутствующий файл выдано предупреждение: %s", warn)
	}
	if len(f.Profiles) != 0 {
		t.Errorf("пустой конфиг не пуст: %#v", f.Profiles)
	}
}

// В файле лежит API-ключ, поэтому он пишется режимом 0600.
func TestSaveWritesOwnerOnly(t *testing.T) {
	path := withConfig(t)
	f, _, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	f.Set("default", Profile{APIKey: "tn_live_секрет"})
	f.Current = "default"
	if err := f.Save(); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := st.Mode().Perm(); perm != 0o600 {
		t.Fatalf("конфиг с ключом записан режимом %04o", perm)
	}

	again, warn, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if warn != "" {
		t.Errorf("свой же файл вызвал предупреждение: %s", warn)
	}
	name, p := again.Get("")
	if name != "default" || p.APIKey != "tn_live_секрет" {
		t.Fatalf("профиль прочитан неверно: %s %#v", name, p)
	}
}

// Слишком широкие права не ошибка, но и молчать о читаемом всеми секрете
// нельзя — иначе о нём узнают не от нас.
func TestLoadWarnsOnLoosePermissions(t *testing.T) {
	path := withConfig(t)
	if err := os.WriteFile(path, []byte("current: default\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, warn, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(warn, "chmod 600") {
		t.Fatalf("о правах 0644 не предупредили: %q", warn)
	}
}

func TestGetFallsBackToDefaultProfile(t *testing.T) {
	withConfig(t)
	f, _, _ := Load()
	name, _ := f.Get("")
	if name != DefaultProfile {
		t.Fatalf("без текущего профиля выбран %q", name)
	}
}

func TestSaveIsAtomic(t *testing.T) {
	path := withConfig(t)
	f, _, _ := Load()
	f.Set("a", Profile{APIKey: "1"})
	if err := f.Save(); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".config-") {
			t.Fatalf("после записи остался временный файл %s", e.Name())
		}
	}
}
