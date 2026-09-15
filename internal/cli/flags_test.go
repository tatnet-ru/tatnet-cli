package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// registeredFlags — все имена флагов дерева команд.
func registeredFlags(t *testing.T) map[string]bool {
	t.Helper()
	names := map[string]bool{}
	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		collect := func(f *pflag.Flag) { names[f.Name] = true }
		cmd.Flags().VisitAll(collect)
		cmd.PersistentFlags().VisitAll(collect)
		for _, sub := range cmd.Commands() {
			walk(sub)
		}
	}
	walk(NewRootCommand("test"))
	return names
}

var optCall = regexp.MustCompile(`opt(?:Str|Int|Bool|StrSlice)\(cmd, "([^"]+)"`)

// Необязательные поля тела ставятся по факту наличия флага — а pflag.Changed
// на НЕЗАРЕГИСТРИРОВАННОМ имени молча отвечает «нет». Опечатка или забытая
// регистрация превращают поле в мёртвое: код выглядит рабочим, значение
// никогда не уезжает, сервер об этом не узнаёт. Ловится только так.
func TestEveryOptionalFieldRefersToARegisteredFlag(t *testing.T) {
	known := registeredFlags(t)

	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	var missing []string
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range optCall.FindAllStringSubmatch(string(src), -1) {
			if !known[m[1]] {
				missing = append(missing, file+": флаг --"+m[1]+" не зарегистрирован ни одной командой")
			}
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("поля тела запроса ссылаются на несуществующие флаги:\n  %s",
			strings.Join(missing, "\n  "))
	}
}

// Обязательные флаги обязаны быть объявлены обязательными, а не проверяться
// сервером: 422 от API вместо понятного «укажите --name» — плохой отказ.
func TestRequiredFlagsAreMarked(t *testing.T) {
	cases := map[string][]string{
		"vm create":     {"name", "image", "plan", "cluster", "user"},
		"pg create":     {"name", "plan"},
		"valkey create": {"name", "plan"},
		"app create":    {"name"},
	}
	root := NewRootCommand("test")
	for path, want := range cases {
		cmd, _, err := root.Find(strings.Split(path, " "))
		if err != nil {
			t.Fatalf("команда %q не найдена: %v", path, err)
		}
		for _, name := range want {
			f := cmd.Flags().Lookup(name)
			if f == nil {
				t.Errorf("%s: нет флага --%s", path, name)
				continue
			}
			if f.Annotations[cobra.BashCompOneRequiredFlag] == nil {
				t.Errorf("%s: флаг --%s не помечен обязательным", path, name)
			}
		}
	}
}
