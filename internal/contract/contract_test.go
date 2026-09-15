package contract

import "testing"

func TestOperationsParsed(t *testing.T) {
	ops, err := Operations()
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) < 100 {
		t.Fatalf("в контракте всего %d операций — похоже, разобрался не тот документ", len(ops))
	}
	for _, op := range ops {
		if op.ID == "" || op.Tag == "" || op.Summary == "" {
			t.Fatalf("операция без обязательных полей: %+v", op)
		}
	}
}

// Шаблонные сегменты обязаны совпадать с конкретными значениями, иначе
// проверка адреса отвергала бы любой реальный вызов с идентификатором.
func TestFindMatchesTemplatedPath(t *testing.T) {
	if _, ok := Find("GET", "/projects/7f3c9c1e-0000-0000-0000-000000000000/vms"); !ok {
		t.Error("адрес со значением вместо шаблона не найден")
	}
	if _, ok := Find("GET", "/vpcs"); !ok {
		t.Error("простой адрес не найден")
	}
	if _, ok := Find("DELETE", "/vpcs"); ok {
		t.Error("найден метод, которого у адреса нет")
	}
	if _, ok := Find("GET", "/projects/abc/nope"); ok {
		t.Error("найден несуществующий адрес")
	}
}

func TestSuggestFindsTypos(t *testing.T) {
	for _, typo := range []string{"/vpc", "/ssh-key", "/certificate"} {
		near := Suggest(typo, 3)
		if len(near) == 0 {
			t.Errorf("для %q не нашлось ни одной подсказки", typo)
		}
	}
}
