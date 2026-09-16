package cli

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// runArgs гоняет дерево команд без сервера: проверяемое сюда не доходит.
func runArgs(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	t.Setenv("TATNET_CONFIG", filepath.Join(t.TempDir(), "config.yaml"))
	t.Setenv("TATNET_API_KEY", "tn_live_тест")
	t.Setenv("TATNET_BASE_URL", "")
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

func groupPaths(t *testing.T) [][]string {
	t.Helper()
	var paths [][]string
	var walk func(c *cobra.Command, prefix []string)
	walk = func(c *cobra.Command, prefix []string) {
		if c.Annotations[groupAnnotation] == "1" && len(prefix) > 0 {
			paths = append(paths, append([]string(nil), prefix...))
		}
		for _, sub := range c.Commands() {
			if sub.Name() == "help" || sub.Name() == "completion" {
				continue
			}
			walk(sub, append(prefix, sub.Name()))
		}
	}
	walk(NewRootCommand("test"), nil)
	if len(paths) == 0 {
		t.Fatal("в дереве не нашлось ни одной группы — тест ничего не проверяет")
	}
	return paths
}

// Опечатка в подкоманде обязана быть отказом.
//
// Cobra по умолчанию печатает на это справку и выходит с нулём: `tatnet dns
// list` в скрипте положил бы в файл текст справки, и `set -e` этого не
// заметил бы. Проверяется каждая группа дерева, а не одна показательная.
func TestUnknownSubcommandIsAnError(t *testing.T) {
	for _, path := range groupPaths(t) {
		name := strings.Join(path, " ")
		t.Run(name, func(t *testing.T) {
			args := append(append([]string(nil), path...), "такой-подкоманды-нет")
			out, _, err := runArgs(t, args...)
			if err == nil {
				t.Fatalf("`tatnet %s такой-подкоманды-нет` завершилась успехом, вывод:\n%s", name, out)
			}
			if !strings.Contains(err.Error(), "такой-подкоманды-нет") {
				t.Errorf("в тексте отказа нет самой опечатки: %v", err)
			}
		})
	}
}

// Голая группа — по-прежнему справка и нулевой код: ломать `tatnet dns`
// починкой опечаток было бы лечением хуже болезни.
func TestBareGroupPrintsHelp(t *testing.T) {
	out, _, err := runArgs(t, "dns")
	if err != nil {
		t.Fatalf("`tatnet dns` вернула ошибку: %v", err)
	}
	if !strings.Contains(out, "Available Commands") && !strings.Contains(out, "zone") {
		t.Errorf("справка не напечатана:\n%s", out)
	}
}

// Подсказка про политику ключа не должна появляться там, где API не звали.
//
// Пустой список ПРОФИЛЕЙ означает ровно то, что профилей нет: отправлять
// человека проверять права ключа — посылать его чинить исправное.
func TestLocalEmptyListHasNoKeyPolicyHint(t *testing.T) {
	_, errOut, err := runArgs(t, "profile", "list")
	if err != nil {
		t.Fatalf("profile list: %v", err)
	}
	if strings.Contains(errOut, "не разрешено их видеть") {
		t.Errorf("локальному списку приписана политика ключа:\n%s", errOut)
	}
}

// Отсутствие корневых сертификатов обязано называться своим именем.
//
// Замер в node:22-slim: пакета ca-certificates нет, и клиент отвечал голым
// «x509: certificate signed by unknown authority» — это читается как «плохой
// сертификат у TatNet» и уводит искать неисправность не там.
func TestUnknownAuthorityNamesTheCause(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	// Отказ рукопожатия тут ожидаем — незачем засорять им вывод теста.
	srv.Config.ErrorLog = log.New(io.Discard, "", 0)
	defer srv.Close()

	for _, args := range [][]string{
		{"project", "list"}, // путь через сгенерированный клиент
		{"api", "/vpcs"},    // путь через прямой вызов
	} {
		name := strings.Join(args, " ")
		t.Run(name, func(t *testing.T) {
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
			if err == nil {
				t.Fatal("самоподписанный сертификат принят без возражений")
			}
			if !strings.Contains(err.Error(), "ca-certificates") {
				t.Errorf("причина не названа, текст: %v", err)
			}
		})
	}
}
