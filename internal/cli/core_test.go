package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestDecodeReturnsAPIErrorWithDetail(t *testing.T) {
	_, err := decode(422, []byte(`{"detail":[{"loc":["body","name"],"msg":"field required"}]}`), nil)
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("ожидалась APIError, получено %T", err)
	}
	if !strings.Contains(apiErr.Error(), "body.name: field required") {
		t.Errorf("причина отказа потеряна: %s", apiErr.Error())
	}
}

func TestDecodeStringDetail(t *testing.T) {
	_, err := decode(403, []byte(`{"detail":"нет прав"}`), nil)
	if err == nil || !strings.Contains(err.Error(), "нет прав") {
		t.Fatalf("строковый detail потерян: %v", err)
	}
}

// Успешный ответ не обязан быть JSON — PEM сертификата приходит текстом.
func TestDecodeNonJSONSuccessBody(t *testing.T) {
	v, err := decode(200, []byte("-----BEGIN CERTIFICATE-----"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if s, ok := v.(string); !ok || !strings.HasPrefix(s, "-----BEGIN") {
		t.Fatalf("тело не сохранено как текст: %#v", v)
	}
}

// Клиент при сетевой ошибке возвращает (nil, err): развернуть такой ответ
// нужно сообщением, а не паникой.
func TestUnwrapNilResponse(t *testing.T) {
	type resp struct{ Body []byte }
	var nilResp *resp
	if _, _, err := unwrap(nilResp, nil); err == nil {
		t.Fatal("пустой ответ без ошибки принят за успех")
	}
	if _, _, err := unwrap(nilResp, errors.New("сеть")); err == nil || err.Error() != "сеть" {
		t.Fatalf("исходная ошибка подменена: %v", err)
	}
}

type fakeResp struct {
	Body []byte
	code int
}

func (f *fakeResp) StatusCode() int { return f.code }

func TestCallReadsBodyAndStatus(t *testing.T) {
	v, err := call(&fakeResp{Body: []byte(`{"id":"x"}`), code: 200}, nil)
	if err != nil {
		t.Fatal(err)
	}
	m, _ := v.(map[string]any)
	if m["id"] != "x" {
		t.Fatalf("тело разобрано неверно: %#v", v)
	}
	if _, err := call(&fakeResp{Body: []byte(`{"detail":"нет"}`), code: 404}, nil); err == nil {
		t.Fatal("статус 404 принят за успех")
	}
}

// Список должен вычитываться целиком: count в конверте — размер страницы,
// а не размер набора, и остановка по нему обрывала бы вывод на первой сотне.
func TestPaginateReadsAllPages(t *testing.T) {
	total := 250
	fetch := func(_ context.Context, offset, limit int) (any, error) {
		data := []any{}
		for i := offset; i < offset+limit && i < total; i++ {
			data = append(data, map[string]any{"id": fmt.Sprint(i)})
		}
		return map[string]any{"count": len(data), "data": data, "limit": limit, "offset": offset}, nil
	}
	all, err := paginate(context.Background(), fetch, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != total {
		t.Fatalf("вычитано %d из %d — список оборван", len(all), total)
	}
}

func TestPaginateRespectsLimit(t *testing.T) {
	fetch := func(_ context.Context, offset, limit int) (any, error) {
		data := []any{}
		for i := 0; i < limit; i++ {
			data = append(data, map[string]any{"id": fmt.Sprint(offset + i)})
		}
		return map[string]any{"data": data}, nil
	}
	all, err := paginate(context.Background(), fetch, 7, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 7 {
		t.Fatalf("--limit не соблюдён: %d", len(all))
	}
}

// Одноимённые ресурсы — обычное дело. Взять первый значит однажды удалить
// не тот, поэтому неоднозначность обязана быть отказом.
func TestResolveRefRejectsAmbiguousName(t *testing.T) {
	list := func(context.Context) ([]any, error) {
		return []any{
			map[string]any{"id": "11111111-1111-1111-1111-111111111111", "name": "web"},
			map[string]any{"id": "22222222-2222-2222-2222-222222222222", "name": "web"},
		}, nil
	}
	_, err := resolveRef(context.Background(), "ВМ", "web", list, "name")
	if err == nil || !strings.Contains(err.Error(), "несколько") {
		t.Fatalf("неоднозначное имя принято: %v", err)
	}
}

func TestResolveRefPassesThroughID(t *testing.T) {
	id := "33333333-3333-3333-3333-333333333333"
	called := false
	list := func(context.Context) ([]any, error) { called = true; return nil, nil }
	got, err := resolveRef(context.Background(), "ВМ", id, list, "name")
	if err != nil || got != id {
		t.Fatalf("идентификатор не пропущен как есть: %q %v", got, err)
	}
	if called {
		t.Error("ради готового идентификатора сходили в API за списком")
	}
}

func TestResolveRefUnknownNameListsCandidates(t *testing.T) {
	list := func(context.Context) ([]any, error) {
		return []any{map[string]any{"id": "44444444-4444-4444-4444-444444444444", "name": "api"}}, nil
	}
	_, err := resolveRef(context.Background(), "ВМ", "нет-такой", list, "name")
	if err == nil || !strings.Contains(err.Error(), "api") {
		t.Fatalf("отказ не подсказал, что есть: %v", err)
	}
}

func TestParseInterface(t *testing.T) {
	iface, err := parseInterface("type=vpc,vpc_id=abc,floating_ip=true")
	if err != nil {
		t.Fatal(err)
	}
	if iface.Type != "vpc" || iface.VpcId == nil || *iface.VpcId != "abc" || iface.FloatingIp == nil || !*iface.FloatingIp {
		t.Fatalf("разобрано неверно: %+v", iface)
	}
	if _, err := parseInterface("vpc_id=abc"); err == nil {
		t.Error("интерфейс без type принят")
	}
	if _, err := parseInterface("type=vpc,floating_ip=да"); err == nil {
		t.Error("нелогическое значение флага принято")
	}
}
