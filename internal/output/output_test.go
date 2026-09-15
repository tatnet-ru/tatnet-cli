package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func render(t *testing.T, f Format, fn func(p Printer) error) string {
	t.Helper()
	var buf bytes.Buffer
	if err := fn(Printer{Out: &buf, Format: f}); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// Пустая выдача должна говорить, что она пустая: иначе «ничего не вывелось»
// неотличимо от «команда не отработала».
func TestEmptyListSaysSo(t *testing.T) {
	out := render(t, Table, func(p Printer) error { return p.List(nil, []Column{Col("имя", "name")}) })
	if !strings.Contains(out, "ничего не найдено") {
		t.Fatalf("пустой список напечатан пустотой: %q", out)
	}
}

func TestTableShowsDeclaredColumns(t *testing.T) {
	items := []any{map[string]any{"name": "web", "status": "running", "secret": "x"}}
	out := render(t, Table, func(p Printer) error {
		return p.List(items, []Column{Col("имя", "name"), Col("статус", "status")})
	})
	if !strings.Contains(out, "ИМЯ") || !strings.Contains(out, "web") || !strings.Contains(out, "running") {
		t.Fatalf("таблица неполна: %q", out)
	}
	if strings.Contains(out, "secret") {
		t.Errorf("в таблицу попало необъявленное поле: %q", out)
	}
}

func TestWideColumnsHiddenByDefault(t *testing.T) {
	items := []any{map[string]any{"name": "web", "id": "abc"}}
	cols := []Column{Col("имя", "name"), WideCol("id", "id")}
	narrow := render(t, Table, func(p Printer) error { return p.List(items, cols) })
	wide := render(t, Wide, func(p Printer) error { return p.List(items, cols) })
	if strings.Contains(narrow, "abc") {
		t.Errorf("wide-колонка показана без -o wide: %q", narrow)
	}
	if !strings.Contains(wide, "abc") {
		t.Errorf("wide-колонка не показана при -o wide: %q", wide)
	}
}

// Машинные форматы обязаны отдавать ровно то, что пришло от API, — на них
// строят скрипты.
func TestJSONIsUnmodified(t *testing.T) {
	items := []any{map[string]any{"name": "web", "nested": map[string]any{"a": float64(1)}}}
	out := render(t, JSON, func(p Printer) error { return p.List(items, nil) })
	var back []any
	if err := json.Unmarshal([]byte(out), &back); err != nil {
		t.Fatalf("вывод -o json не разбирается: %v", err)
	}
	if len(back) != 1 {
		t.Fatalf("потеряны элементы: %s", out)
	}
}

// Отсутствующее поле — прочерк: пустая ячейка читается как «поле пустое».
func TestValueMarksMissingField(t *testing.T) {
	obj := map[string]any{"plan": map[string]any{"name": "pg1.2g"}, "empty": ""}
	if got := Value(obj, "plan.name"); got != "pg1.2g" {
		t.Errorf("вложенное поле не прочитано: %q", got)
	}
	if got := Value(obj, "нет.такого"); got != "-" {
		t.Errorf("отсутствующее поле показано как %q", got)
	}
	if got := Value(obj, "empty"); got != "" {
		t.Errorf("пустая строка подменена на %q", got)
	}
}

func TestStringifyCollections(t *testing.T) {
	if got := Stringify([]any{"a", "b"}); got != "a,b" {
		t.Errorf("массив: %q", got)
	}
	if got := Stringify([]any{}); got != "-" {
		t.Errorf("пустой массив: %q", got)
	}
	if got := Stringify(float64(3)); got != "3" {
		t.Errorf("целое напечатано как %q", got)
	}
	if got := Stringify(true); got != "да" {
		t.Errorf("булево: %q", got)
	}
}

// Подтверждение в машинном формате обязано оставаться разбираемым.
func TestMessageStaysParsableInJSON(t *testing.T) {
	out := render(t, JSON, func(p Printer) error { return p.Message("готово: %d", 2) })
	var m map[string]string
	if err := json.Unmarshal([]byte(out), &m); err != nil || m["message"] != "готово: 2" {
		t.Fatalf("сообщение не разбирается как JSON: %q", out)
	}
}

func TestParseFormatRejectsUnknown(t *testing.T) {
	if _, err := ParseFormat("csv"); err == nil {
		t.Fatal("неизвестный формат принят")
	}
	for _, s := range []string{"table", "JSON", " yaml ", "wide"} {
		if _, err := ParseFormat(s); err != nil {
			t.Errorf("формат %q отвергнут: %v", s, err)
		}
	}
}
