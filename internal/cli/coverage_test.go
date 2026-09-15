package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/tatnet-ru/tatnet-cli/internal/contract"
)

// boundOps обходит дерево команд и собирает объявленные операции.
func boundOps(t *testing.T) map[string][]string {
	t.Helper()
	bound := map[string][]string{}
	var walk func(cmd *cobra.Command, path string)
	walk = func(cmd *cobra.Command, path string) {
		full := strings.TrimSpace(path + " " + cmd.Name())
		for _, op := range CommandOps(cmd.Annotations) {
			bound[op] = append(bound[op], full)
		}
		for _, sub := range cmd.Commands() {
			walk(sub, full)
		}
	}
	walk(NewRootCommand("test"), "")
	return bound
}

// Любая объявленная командой операция обязана существовать в контракте:
// переименовали операцию в API — CLI должен падать здесь, а не у клиента.
func TestEveryBoundOperationExistsInContract(t *testing.T) {
	ops, err := contract.Operations()
	if err != nil {
		t.Fatal(err)
	}
	known := map[string]bool{}
	for _, op := range ops {
		known[op.ID] = true
	}
	for id, cmds := range boundOps(t) {
		if !known[id] {
			t.Errorf("команда %s ссылается на операцию %q, которой нет в контракте",
				strings.Join(cmds, ", "), id)
		}
	}
}

// Покрытый раздел покрыт целиком: новая операция в нём валит тест, а не
// тихо остаётся недоступной из CLI.
func TestCoveredTagsAreCoveredCompletely(t *testing.T) {
	ops, err := contract.Operations()
	if err != nil {
		t.Fatal(err)
	}
	bound := boundOps(t)

	var missing []string
	for _, op := range ops {
		if _, deferred := deferredTags[op.Tag]; deferred {
			continue
		}
		if len(bound[op.ID]) == 0 {
			missing = append(missing, op.Tag+" → "+op.ID+" ("+op.Method+" "+op.Path+")")
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("в покрытых разделах не выведено в CLI %d операций:\n  %s",
			len(missing), strings.Join(missing, "\n  "))
	}
}

// Отложенный раздел не покрыт частично: наполовину выведенный раздел — это
// список, которому уже нельзя верить.
func TestDeferredTagsAreNotPartiallyCovered(t *testing.T) {
	ops, err := contract.Operations()
	if err != nil {
		t.Fatal(err)
	}
	bound := boundOps(t)
	for _, op := range ops {
		if _, deferred := deferredTags[op.Tag]; !deferred {
			continue
		}
		if cmds := bound[op.ID]; len(cmds) > 0 {
			t.Errorf("раздел %q помечен отложенным, но операция %s выведена командой %s — "+
				"уберите раздел из deferredTags и покройте его целиком",
				op.Tag, op.ID, strings.Join(cmds, ", "))
		}
	}
}

// Каждый отложенный раздел должен существовать в контракте: раздел, который
// переименовали в API, иначе остался бы вечным исключением ни для чего.
func TestDeferredTagsExistInContract(t *testing.T) {
	ops, err := contract.Operations()
	if err != nil {
		t.Fatal(err)
	}
	tags := map[string]bool{}
	for _, op := range ops {
		tags[op.Tag] = true
	}
	for tag := range deferredTags {
		if !tags[tag] {
			t.Errorf("раздел %q отложен, но в контракте его нет", tag)
		}
	}
}

// Вшитый контракт обязан совпадать с контрактом той версии tatnet-go, на
// которой собран CLI. Иначе `tatnet api` проверял бы адреса по одному
// документу, а клиент ходил бы по другому.
func TestEmbeddedContractMatchesClientModule(t *testing.T) {
	out, err := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", "github.com/tatnet-ru/tatnet-go").Output()
	if err != nil {
		t.Skipf("модуль клиента недоступен: %v", err)
	}
	dir := strings.TrimSpace(string(out))
	if dir == "" {
		t.Skip("каталог модуля клиента пуст")
	}
	theirs, err := os.ReadFile(filepath.Join(dir, "openapi", "v1.json"))
	if err != nil {
		t.Fatalf("не прочитан контракт клиента: %v", err)
	}
	if string(theirs) != string(contract.Raw) {
		t.Fatal("вшитый контракт разошёлся с контрактом tatnet-go: " +
			"выполните scripts/sync-contract.sh и пересоберите")
	}
}

// Счётчик покрытия — не гейт, а видимая цифра в выводе тестов.
func TestCoverageSummary(t *testing.T) {
	ops, err := contract.Operations()
	if err != nil {
		t.Fatal(err)
	}
	bound := boundOps(t)
	covered := 0
	for _, op := range ops {
		if len(bound[op.ID]) > 0 {
			covered++
		}
	}
	t.Logf("операций в контракте: %d, выведено командами: %d, отложено разделов: %d",
		len(ops), covered, len(deferredTags))
}
