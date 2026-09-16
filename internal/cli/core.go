package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httputil"
	"os"
	"strings"
	"time"

	"github.com/tatnet-ru/tatnet-go/tatnet"

	"github.com/tatnet-ru/tatnet-cli/internal/config"
	"github.com/tatnet-ru/tatnet-cli/internal/output"
)

// Env — всё, что команде нужно помимо её собственных флагов.
type Env struct {
	Printer output.Printer
	Cfg     *config.File

	ProfileName string
	Profile     config.Profile

	APIKey  string
	BaseURL string
	Project string

	Timeout time.Duration
	Debug   bool

	client *tatnet.ClientWithResponses
	// Отдельный клиент для потоков: у него нет общего таймаута запроса.
	streamClient *tatnet.ClientWithResponses
}

// ErrNoAPIKey — ключа нет нигде: ни во флаге, ни в окружении, ни в профиле.
type ErrNoAPIKey struct{ Profile string }

func (e ErrNoAPIKey) Error() string {
	return fmt.Sprintf("нет API-ключа для профиля %q.\n"+
		"Задайте его одним из способов:\n"+
		"  tatnet auth login            — сохранить ключ в профиль\n"+
		"  export TATNET_API_KEY=tn_…   — на время сессии\n"+
		"  tatnet --api-key tn_… …      — на один вызов\n"+
		"Ключ создаётся в панели: https://min.tatnet.ru/api-keys", e.Profile)
}

// Client собирает клиент по реквизитам профиля. Один на вызов команды.
func (e *Env) Client() (*tatnet.ClientWithResponses, error) {
	if e.client != nil {
		return e.client, nil
	}
	c, err := e.newClient(e.Timeout)
	if err != nil {
		return nil, err
	}
	e.client = c
	return c, nil
}

// StreamClient — клиент для ДОЛГИХ ответов: лога сборки и прочего, что
// читается потоком.
//
// Общий таймаут запроса тут вреден: он считается на весь ответ целиком, а
// поток живёт столько, сколько идёт сборка. С `--timeout 1m` лог обрывался
// на минуте словами «context deadline exceeded» — то есть здоровый поток
// объявлялся сбоем. Ограничение по времени у потока одно и правильное:
// отмена контекста (Ctrl+C) и конец самой сборки.
func (e *Env) StreamClient() (*tatnet.ClientWithResponses, error) {
	if e.streamClient != nil {
		return e.streamClient, nil
	}
	c, err := e.newClient(0)
	if err != nil {
		return nil, err
	}
	e.streamClient = c
	return c, nil
}

func (e *Env) newClient(timeout time.Duration) (*tatnet.ClientWithResponses, error) {
	if strings.TrimSpace(e.APIKey) == "" {
		return nil, ErrNoAPIKey{Profile: e.ProfileName}
	}
	hc := &http.Client{Timeout: timeout}
	if e.Debug {
		hc.Transport = debugTransport{next: http.DefaultTransport}
	}
	opts := []tatnet.ClientOption{tatnet.WithHTTPClient(hc)}
	if e.BaseURL != "" && e.BaseURL != tatnet.DefaultBaseURL {
		c, err := tatnet.NewClientWithResponses(e.BaseURL,
			append([]tatnet.ClientOption{tatnet.WithAPIKey(e.APIKey)}, opts...)...)
		if err != nil {
			return nil, err
		}
		return c, nil
	}
	c, err := tatnet.New(e.APIKey, opts...)
	if err != nil {
		return nil, err
	}
	return c, nil
}

// RequireProject возвращает проект для команд, которые без него бессмысленны.
func (e *Env) RequireProject(ctx context.Context) (string, error) {
	if strings.TrimSpace(e.Project) == "" {
		return "", fmt.Errorf("не задан проект.\n" +
			"Укажите его флагом -p/--project, переменной TATNET_PROJECT\n" +
			"или сохраните в профиль: tatnet profile set-project <проект>.\n" +
			"Список проектов: tatnet project list")
	}
	return e.ResolveProject(ctx, e.Project)
}

// APIError — отказ, пришедший от API.
type APIError struct {
	Status int
	Detail string
	Body   []byte
}

func (e *APIError) Error() string {
	msg := e.Detail
	if msg == "" {
		msg = strings.TrimSpace(string(e.Body))
	}
	if msg == "" {
		msg = http.StatusText(e.Status)
	}
	switch e.Status {
	case http.StatusUnauthorized:
		return fmt.Sprintf("401 — ключ не принят: %s\nПроверьте: tatnet auth status", msg)
	case http.StatusForbidden:
		return fmt.Sprintf("403 — ключу не хватает прав: %s\n"+
			"Политика ключа задаётся при его создании и ограничена правами выдавшего участника.", msg)
	case http.StatusNotFound:
		return fmt.Sprintf("404 — не найдено: %s", msg)
	}
	return fmt.Sprintf("%d — %s", e.Status, msg)
}

// decode разбирает ответ типизированного клиента.
//
// Тело берётся сырым, а не из JSON200-полей: рендер таблиц всё равно работает
// по разобранному JSON, а так один helper обслуживает все 24 списочных типа.
func decode(status int, body []byte, err error) (any, error) {
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, &APIError{Status: status, Detail: detailOf(body), Body: body}
	}
	if len(body) == 0 {
		return nil, nil
	}
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		// Не всякий успешный ответ — JSON (например, PEM сертификата).
		return string(body), nil
	}
	return v, nil
}

// check — decode для команд, которым тело ответа не нужно.
func check(status int, body []byte, err error) error {
	_, err = decode(status, body, err)
	return err
}

// detailOf вытаскивает человекочитаемую причину из тела отказа FastAPI:
// либо строка в `detail`, либо список ошибок валидации.
func detailOf(body []byte) string {
	var envelope struct {
		Detail json.RawMessage `json:"detail"`
	}
	if json.Unmarshal(body, &envelope) != nil || len(envelope.Detail) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(envelope.Detail, &s) == nil {
		return s
	}
	var items []struct {
		Loc []any  `json:"loc"`
		Msg string `json:"msg"`
	}
	if json.Unmarshal(envelope.Detail, &items) == nil && len(items) > 0 {
		parts := make([]string, 0, len(items))
		for _, it := range items {
			where := make([]string, 0, len(it.Loc))
			for _, l := range it.Loc {
				where = append(where, output.Stringify(l))
			}
			parts = append(parts, strings.Join(where, ".")+": "+it.Msg)
		}
		return strings.Join(parts, "; ")
	}
	return strings.TrimSpace(string(envelope.Detail))
}

// items достаёт массив из страничного конверта {count,data,limit,offset}.
//
// ⚠ count — размер СТРАНИЦЫ, а не всего набора; сколько всего, API не
// сообщает. Поэтому конец данных определяется короткой страницей, а не
// сравнением с count.
func items(v any) []any {
	switch t := v.(type) {
	case nil:
		return nil
	case []any:
		return t
	case map[string]any:
		if d, ok := t["data"].([]any); ok {
			return d
		}
		return []any{t}
	default:
		return []any{t}
	}
}

// fetchPage — один запрос страницы.
type fetchPage func(ctx context.Context, offset, limit int) (any, error)

// paginate вычитывает список целиком.
//
// CLI, отдающий первую страницу как весь ответ, врёт тихо: у `list` нет
// признака обрыва, и скрипт поверх него примет 100 строк за всё. Поэтому
// по умолчанию команда идёт до конца, а --limit ограничивает явно.
func paginate(ctx context.Context, fetch fetchPage, limit, pageSize int) ([]any, error) {
	const maxPage = 100
	if pageSize <= 0 || pageSize > maxPage {
		pageSize = maxPage
	}
	var out []any
	for offset := 0; ; offset += pageSize {
		size := pageSize
		if limit > 0 && limit-len(out) < size {
			size = limit - len(out)
		}
		v, err := fetch(ctx, offset, size)
		if err != nil {
			return nil, err
		}
		page := items(v)
		out = append(out, page...)
		if len(page) < size || (limit > 0 && len(out) >= limit) {
			return out, nil
		}
	}
}

// debugTransport печатает запрос и ответ в stderr, вырезая ключ.
type debugTransport struct{ next http.RoundTripper }

func (d debugTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	redacted := req.Clone(req.Context())
	if redacted.Header.Get("Authorization") != "" {
		redacted.Header.Set("Authorization", "Bearer «вырезано»")
	}
	if dump, err := httputil.DumpRequestOut(redacted, true); err == nil {
		fmt.Fprintf(os.Stderr, "→ %s\n", dump)
	}
	resp, err := d.next.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	if dump, err := httputil.DumpResponse(resp, true); err == nil {
		fmt.Fprintf(os.Stderr, "← %s\n", dump)
	}
	return resp, nil
}
