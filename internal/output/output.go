// Package output печатает ответы API в одном из форматов: таблица, json, yaml.
//
// Таблицы строятся по объявленным колонкам, а значения берутся из РАЗОБРАННОГО
// JSON, а не из типов клиента: иначе на каждый из 24 списочных типов пришлось
// бы писать свой форматтер, и они бы разъезжались.
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"gopkg.in/yaml.v3"
)

// Format — как печатать ответ.
type Format string

const (
	Table Format = "table"
	JSON  Format = "json"
	YAML  Format = "yaml"
	Wide  Format = "wide"
)

// ParseFormat разбирает значение флага -o.
func ParseFormat(s string) (Format, error) {
	switch Format(strings.ToLower(strings.TrimSpace(s))) {
	case Table:
		return Table, nil
	case JSON:
		return JSON, nil
	case YAML:
		return YAML, nil
	case Wide:
		return Wide, nil
	default:
		return "", fmt.Errorf("неизвестный формат %q: допустимы table, wide, json, yaml", s)
	}
}

// Column — колонка таблицы. Path — путь в JSON через точку (`plan.name`).
// Wide-колонки печатаются только при -o wide.
type Column struct {
	Title string
	Path  string
	Wide  bool
}

// Col — обычная колонка.
func Col(title, path string) Column { return Column{Title: title, Path: path} }

// WideCol — колонка, видимая только при -o wide.
func WideCol(title, path string) Column { return Column{Title: title, Path: path, Wide: true} }

// Printer печатает результат одной команды.
type Printer struct {
	Out    io.Writer
	Err    io.Writer
	Format Format
}

// List печатает массив объектов, ПОЛУЧЕННЫЙ ОТ API. Пустой список печатается
// заметной строкой, а не пустотой: «ничего не вывелось» иначе неотличимо от
// «команда не отработала».
func (p Printer) List(items []any, cols []Column) error {
	return p.list(items, cols, true)
}

// ListLocal — то же для данных, которые в API не ходили: профили из конфига
// и прочее местное. Подсказка про политику ключа там была бы ложью — пустой
// список профилей означает ровно то, что профилей нет, и отправлять человека
// проверять права значит послать его чинить исправное.
func (p Printer) ListLocal(items []any, cols []Column) error {
	return p.list(items, cols, false)
}

func (p Printer) list(items []any, cols []Column, remote bool) error {
	if items == nil {
		// Пустой СПИСОК, а не null: иначе `... -o json | jq length`
		// спотыкается ровно там, где записей не оказалось.
		items = []any{}
	}
	switch p.Format {
	case JSON:
		return p.writeJSON(items)
	case YAML:
		return p.writeYAML(items)
	}
	if len(items) == 0 {
		if _, err := fmt.Fprintln(p.Out, "ничего не найдено"); err != nil {
			return err
		}
		// Пустой список у /v1 означает ещё и «ключу не видно»: коллекции
		// фильтруются по политике ключа, а не отвечают 403 — так задумано,
		// чтобы ключ с привязкой к ресурсам видел свой срез. Значит, «нет
		// записей» и «не смогли узнать» приходят одинаково, и CLI не вправе
		// выдавать второе за первое. Подсказка идёт в stderr, чтобы не
		// попасть в разбор вывода.
		if remote && p.Err != nil {
			fmt.Fprintln(p.Err, "Если записи ожидались — пустой список приходит и тогда, "+
				"когда ключу не разрешено их видеть: tatnet auth status")
		}
		return nil
	}
	return p.table(items, cols)
}

// Object печатает один объект: в табличном режиме — вертикально, парами
// «поле: значение». Однострочная широкая таблица нечитаема.
func (p Printer) Object(obj any, cols []Column) error {
	switch p.Format {
	case JSON:
		return p.writeJSON(obj)
	case YAML:
		return p.writeYAML(obj)
	}
	tw := tabwriter.NewWriter(p.Out, 0, 0, 2, ' ', 0)
	for _, c := range cols {
		if c.Wide && p.Format != Wide {
			continue
		}
		fmt.Fprintf(tw, "%s\t%s\n", c.Title, Value(obj, c.Path))
	}
	return tw.Flush()
}

// Raw печатает произвольное значение: json/yaml как просили, иначе — как
// есть (для текстовых ответов вроде PEM-сертификата).
func (p Printer) Raw(v any) error {
	switch p.Format {
	case YAML:
		return p.writeYAML(v)
	case JSON:
		return p.writeJSON(v)
	}
	if s, ok := v.(string); ok {
		_, err := fmt.Fprintln(p.Out, strings.TrimRight(s, "\n"))
		return err
	}
	return p.writeJSON(v)
}

// Message печатает короткое человекочитаемое подтверждение. В машинных
// форматах оно превращается в объект, чтобы вывод оставался разбираемым.
func (p Printer) Message(format string, args ...any) error {
	msg := fmt.Sprintf(format, args...)
	switch p.Format {
	case JSON, YAML:
		return p.Raw(map[string]string{"message": msg})
	}
	_, err := fmt.Fprintln(p.Out, msg)
	return err
}

func (p Printer) table(items []any, cols []Column) error {
	tw := tabwriter.NewWriter(p.Out, 0, 0, 3, ' ', 0)
	visible := make([]Column, 0, len(cols))
	for _, c := range cols {
		if c.Wide && p.Format != Wide {
			continue
		}
		visible = append(visible, c)
	}
	titles := make([]string, len(visible))
	for i, c := range visible {
		titles[i] = strings.ToUpper(c.Title)
	}
	fmt.Fprintln(tw, strings.Join(titles, "\t"))
	for _, it := range items {
		cells := make([]string, len(visible))
		for i, c := range visible {
			cells[i] = Value(it, c.Path)
		}
		fmt.Fprintln(tw, strings.Join(cells, "\t"))
	}
	return tw.Flush()
}

func (p Printer) writeJSON(v any) error {
	enc := json.NewEncoder(p.Out)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

func (p Printer) writeYAML(v any) error {
	enc := yaml.NewEncoder(p.Out)
	enc.SetIndent(2)
	if err := enc.Encode(v); err != nil {
		return err
	}
	return enc.Close()
}

// Value достаёт значение по пути через точку и приводит его к строке для
// таблицы. Отсутствующее поле — прочерк, а не пустая ячейка: пустая строка в
// таблице читается как «поле есть и оно пустое».
func Value(obj any, path string) string {
	v, ok := lookup(obj, path)
	if !ok || v == nil {
		return "-"
	}
	return Stringify(v)
}

func lookup(obj any, path string) (any, bool) {
	cur := obj
	for _, part := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = m[part]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

// Stringify превращает разобранное JSON-значение в одну строку таблицы.
func Stringify(v any) string {
	switch t := v.(type) {
	case nil:
		return "-"
	case string:
		return t
	case bool:
		if t {
			return "да"
		}
		return "нет"
	case float64:
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	case []any:
		if len(t) == 0 {
			return "-"
		}
		parts := make([]string, len(t))
		for i, e := range t {
			parts[i] = Stringify(e)
		}
		return strings.Join(parts, ",")
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			parts = append(parts, k+"="+Stringify(t[k]))
		}
		return strings.Join(parts, " ")
	default:
		return fmt.Sprint(v)
	}
}
