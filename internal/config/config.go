// Package config хранит профили CLI: адрес API, ключ и проект по умолчанию.
//
// Файл лежит по $TATNET_CONFIG, иначе $XDG_CONFIG_HOME/tatnet/config.yaml,
// иначе ~/.config/tatnet/config.yaml. Ключ — секрет, поэтому файл пишется
// режимом 0600, а слишком широкие права на чтении не молчат.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Profile — один набор реквизитов. Profile.Project необязателен: он лишь
// избавляет от -p в каждой команде.
type Profile struct {
	APIKey  string `yaml:"api_key,omitempty"`
	BaseURL string `yaml:"base_url,omitempty"`
	Project string `yaml:"project,omitempty"`
}

// File — содержимое конфига целиком.
type File struct {
	Current  string             `yaml:"current,omitempty"`
	Profiles map[string]Profile `yaml:"profiles,omitempty"`

	// path запоминается при чтении, чтобы Save писал туда же, откуда читал,
	// даже если переменные окружения успели измениться.
	path string `yaml:"-"`
}

// DefaultProfile — имя профиля, когда пользователь не назвал другой.
const DefaultProfile = "default"

// Path возвращает путь к конфигу по переменным окружения.
func Path() (string, error) {
	if p := os.Getenv("TATNET_CONFIG"); p != "" {
		return p, nil
	}
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, "tatnet", "config.yaml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("не удалось определить домашний каталог: %w", err)
	}
	return filepath.Join(home, ".config", "tatnet", "config.yaml"), nil
}

// Load читает конфиг. Отсутствие файла — не ошибка: у CLI, который целиком
// настраивается переменными окружения, конфига может не быть вовсе.
//
// Второе возвращаемое значение — предупреждение о правах доступа. Оно не
// ошибка (работать можно), но и молчать о секрете, читаемом всей машиной,
// нельзя.
func Load() (*File, string, error) {
	path, err := Path()
	if err != nil {
		return nil, "", err
	}
	f := &File{Profiles: map[string]Profile{}, path: path}

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return f, "", nil
	}
	if err != nil {
		return nil, "", fmt.Errorf("не удалось прочитать %s: %w", path, err)
	}
	if err := yaml.Unmarshal(data, f); err != nil {
		return nil, "", fmt.Errorf("не удалось разобрать %s: %w", path, err)
	}
	if f.Profiles == nil {
		f.Profiles = map[string]Profile{}
	}
	f.path = path

	var warn string
	if st, err := os.Stat(path); err == nil && st.Mode().Perm()&0o077 != 0 {
		warn = fmt.Sprintf("%s доступен не только владельцу (%04o); в нём лежит API-ключ — chmod 600 %s",
			path, st.Mode().Perm(), path)
	}
	return f, warn, nil
}

// Save пишет конфиг атомарно и режимом 0600.
//
// Через временный файл рядом с целевым, потому что прерванная запись прямо
// в config.yaml оставила бы пользователя без реквизитов вообще.
func (f *File) Save() error {
	path := f.path
	if path == "" {
		var err error
		if path, err = Path(); err != nil {
			return err
		}
		f.path = path
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("не удалось создать каталог конфига: %w", err)
	}
	data, err := yaml.Marshal(f)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*.yaml")
	if err != nil {
		return fmt.Errorf("не удалось создать временный файл: %w", err)
	}
	defer os.Remove(tmp.Name())

	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// Get возвращает профиль по имени; пустое имя означает текущий.
func (f *File) Get(name string) (string, Profile) {
	if name == "" {
		name = f.Current
	}
	if name == "" {
		name = DefaultProfile
	}
	return name, f.Profiles[name]
}

// Set сохраняет профиль в памяти; записать на диск — дело Save.
func (f *File) Set(name string, p Profile) {
	if f.Profiles == nil {
		f.Profiles = map[string]Profile{}
	}
	f.Profiles[name] = p
}

// Names — имена профилей в порядке, пригодном для вывода.
func (f *File) Names() []string {
	out := make([]string, 0, len(f.Profiles))
	for n := range f.Profiles {
		out = append(out, n)
	}
	return out
}
