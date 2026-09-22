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

	// Двойники: у операции над ресурсом проекта есть две формы адреса — через
	// проект (/projects/{project_id}/<вид>/…) и плоская (/<вид>/…), с одним и
	// тем же смыслом и одной политикой (api#1060). CLI нужна ровно одна из
	// них, и держать вторую «выведенной» ради гейта значило бы плодить
	// мёртвые команды. Поэтому операция считается покрытой, если выведен её
	// двойник. НОВАЯ возможность двойника не имеет и по-прежнему валит тест.
	byKey := map[string]contract.Operation{}
	for _, op := range ops {
		byKey[op.Method+" "+op.Path] = op
	}
	coveredViaTwin := func(op contract.Operation) bool {
		twin, ok := byKey[op.Method+" "+twinPath(op.Path)]
		return ok && len(bound[twin.ID]) > 0
	}

	var missing []string
	for _, op := range ops {
		if _, deferred := deferredTags[op.Tag]; deferred {
			continue
		}
		if len(bound[op.ID]) == 0 && !coveredViaTwin(op) {
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

// flatKinds — виды, у которых api держит плоские зеркала (api#1060):
// приложения (#1061) и остальные семь (#1099). Список явный, а не «любой
// путь под /projects/{project_id}»: двойником считаем только то, что api
// зеркалит нарочно, иначе будущий скоупный путь, у которого случайно
// найдётся плоский однофамилец с другим смыслом, молча засчитался бы
// покрытым.
var flatKinds = []string{
	"apps", "vms", "functions", "volumes",
	"pg-clusters", "valkey-clusters", "kubernetes-clusters", "load-balancers",
}

// twinPath — адрес-двойник: скоупный путь ↔ плоский. Для путей без
// двойника возвращает пустую строку, которая ни с чем не совпадёт.
// Двойник засчитывается, только если он есть в контракте (byKey), поэтому
// /load-balancers/regions не станет «двойником» несуществующего пути.
func twinPath(p string) string {
	const project = "/projects/{project_id}"
	for _, k := range flatKinds {
		scoped := project + "/" + k
		switch {
		case p == scoped || strings.HasPrefix(p, scoped+"/"):
			return "/" + k + strings.TrimPrefix(p, scoped)
		case p == "/"+k || strings.HasPrefix(p, "/"+k+"/"):
			return project + p
		}
	}
	return ""
}

func TestTwinPath(t *testing.T) {
	cases := map[string]string{
		"/projects/{project_id}/apps":                            "/apps",
		"/projects/{project_id}/apps/{app_id}/env":               "/apps/{app_id}/env",
		"/apps/{app_id}/jobs/{job_id}/run":                       "/projects/{project_id}/apps/{app_id}/jobs/{job_id}/run",
		"/apps":                                                  "/projects/{project_id}/apps",
		"/projects/{project_id}/vms/{vm_id}":                     "/vms/{vm_id}",
		"/vms/{vm_id}/start":                                     "/projects/{project_id}/vms/{vm_id}/start",
		"/pg-clusters":                                           "/projects/{project_id}/pg-clusters",
		"/projects/{project_id}/valkey-clusters/{cluster_id}/ca": "/valkey-clusters/{cluster_id}/ca",
		// Не-двойники: аккаунтные пути и виды без зеркал.
		"/domains":                         "",
		"/postgres/regions":                "",
		"/projects/{project_id}":           "",
		"/projects/{project_id}/dns-zones": "",
	}
	for in, want := range cases {
		if got := twinPath(in); got != want {
			t.Errorf("twinPath(%q) = %q, ждали %q", in, got, want)
		}
	}
}
