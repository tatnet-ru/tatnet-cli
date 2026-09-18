package cli

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/tatnet-ru/tatnet-go/tatnet"

	"github.com/tatnet-ru/tatnet-cli/internal/output"
)

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// IsID — похоже ли значение на идентификатор ресурса.
func IsID(s string) bool { return uuidRe.MatchString(s) }

// lister перечисляет ресурсы, среди которых ищем по имени.
type lister func(ctx context.Context) ([]any, error)

// refKind — что ищем, в формах, которые нужны сообщениям. Строка «не найден»
// одна на все виды была неграмотной для половины из них: «приложение не
// найден», «ВМ не найден», «несколько приложение». Тип не даёт передать вид,
// у которого форм нет: новый ресурс обязан их объявить здесь.
type refKind struct {
	one      string // именительный: «приложение»
	notFound string // причастие в роде: «не найдено»
	many     string // родительный множественного после «несколько»: «приложений»
}

var (
	kindApp     = refKind{"приложение", "не найдено", "приложений"}
	kindEnvVar  = refKind{"переменная", "не найдена", "переменных"}
	kindDomain  = refKind{"домен", "не найден", "доменов"}
	kindJob     = refKind{"задача", "не найдена", "задач"}
	kindZone    = refKind{"зона", "не найдена", "зон"}
	kindProject = refKind{"проект", "не найден", "проектов"}
	kindPg      = refKind{"кластер PostgreSQL", "не найден", "кластеров PostgreSQL"}
	kindValkey  = refKind{"кластер Valkey", "не найден", "кластеров Valkey"}
	kindBucket  = refKind{"бакет", "не найден", "бакетов"}
	kindS3Key   = refKind{"ключ доступа", "не найден", "ключей доступа"}
	kindSSHKey  = refKind{"SSH-ключ", "не найден", "SSH-ключей"}
	kindVM      = refKind{"ВМ", "не найдена", "ВМ"}
)

// resolveRef превращает «id или имя» в id.
//
// Совпадение обязано быть единственным: два ресурса с одним именем — обычное
// дело, и молча взять первый значит однажды удалить не тот. Поля имени
// перечисляются вызывающим, потому что у ВМ это hostname, у бакета — name,
// у зоны — домен.
func resolveRef(ctx context.Context, kind refKind, ref string, list lister, nameFields ...string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", fmt.Errorf("не указан: %s", kind.one)
	}
	if IsID(ref) {
		return ref, nil
	}
	all, err := list(ctx)
	if err != nil {
		return "", fmt.Errorf("не удалось найти %s %q: %w", kind.one, ref, err)
	}
	var matched []any
	for _, it := range all {
		for _, f := range nameFields {
			if output.Value(it, f) == ref {
				matched = append(matched, it)
				break
			}
		}
	}
	switch len(matched) {
	case 1:
		id := output.Value(matched[0], "id")
		if id == "-" {
			return "", fmt.Errorf("у найденного (%s) нет поля id", kind.one)
		}
		return id, nil
	case 0:
		names := make([]string, 0, len(all))
		for _, it := range all {
			for _, f := range nameFields {
				if v := output.Value(it, f); v != "-" {
					names = append(names, v)
					break
				}
			}
		}
		sort.Strings(names)
		hint := "список пуст"
		if len(names) > 0 {
			hint = "есть: " + strings.Join(names, ", ")
		}
		return "", fmt.Errorf("%s %q %s (%s)", kind.one, ref, kind.notFound, hint)
	default:
		ids := make([]string, len(matched))
		for i, m := range matched {
			ids[i] = output.Value(m, "id")
		}
		return "", fmt.Errorf("имя %q носят несколько %s — укажите id: %s",
			ref, kind.many, strings.Join(ids, ", "))
	}
}

// listProjects — все проекты аккаунта.
func (e *Env) listProjects(ctx context.Context) ([]any, error) {
	c, err := e.Client()
	if err != nil {
		return nil, err
	}
	return paginate(ctx, func(ctx context.Context, offset, limit int) (any, error) {
		return call(c.ProjectsListProjectsWithResponse(ctx, &tatnet.ProjectsListProjectsParams{
			Limit: &limit, Offset: &offset,
		}))
	}, 0, 0)
}

// ResolveProject превращает «id или имя проекта» в id.
func (e *Env) ResolveProject(ctx context.Context, ref string) (string, error) {
	return resolveRef(ctx, kindProject, ref, e.listProjects, "name")
}
