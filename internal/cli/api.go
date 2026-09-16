package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tatnet-ru/tatnet-cli/internal/contract"
	"github.com/tatnet-ru/tatnet-cli/internal/output"
)

func newAPICommand(env *Env) *cobra.Command {
	var (
		method     string
		fields     []string
		rawFields  []string
		headers    []string
		input      string
		listOps    bool
		allowUnkn  bool
		includeHdr bool
	)

	cmd := &cobra.Command{
		Use:   "api <путь>",
		Short: "Прямой вызов любой операции /v1",
		Long: "Вызывает произвольный адрес публичного API с ключом текущего профиля.\n\n" +
			"Через эту команду доступны ВСЕ операции контракта, включая те, для\n" +
			"которых своей команды ещё нет: балансировщики, сети, Kubernetes,\n" +
			"функции, тома.\n\n" +
			"Адрес проверяется по вшитому контракту ДО отправки: опечатка в пути\n" +
			"иначе вернула бы 404, неотличимый от «ресурса нет».\n\n" +
			"Примеры:\n" +
			"  tatnet api /vpcs\n" +
			"  tatnet api /floating-ips -X POST -f name=fip-1 -f cluster_id=…\n" +
			"  tatnet api /projects/<id>/load-balancers\n" +
			"  tatnet api --list load-balancers",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if listOps {
				pattern := ""
				if len(args) == 1 {
					pattern = args[0]
				}
				return listOperations(cmd, env, pattern)
			}
			if len(args) != 1 {
				return fmt.Errorf("укажите путь, например /vpcs (или --list, чтобы посмотреть доступные)")
			}

			path := normalizePath(args[0])
			body, err := buildBody(cmd, fields, rawFields, input)
			if err != nil {
				return err
			}
			if !cmd.Flags().Changed("method") && body != nil {
				method = http.MethodPost
			}
			method = strings.ToUpper(method)

			if op, ok := contract.Find(method, path); ok {
				if env.Debug {
					fmt.Fprintf(cmd.ErrOrStderr(), "операция: %s (%s)\n", op.ID, op.Summary)
				}
			} else if !allowUnkn {
				return unknownPathError(method, path)
			}

			status, respHeaders, respBody, err := env.rawRequest(cmd, method, path, body, headers)
			if err != nil {
				return err
			}
			if includeHdr {
				fmt.Fprintf(cmd.OutOrStdout(), "%d %s\n", status, http.StatusText(status))
				for k, vals := range respHeaders {
					for _, v := range vals {
						fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", k, v)
					}
				}
				fmt.Fprintln(cmd.OutOrStdout())
			}
			if status >= 400 {
				return &APIError{Status: status, Detail: detailOf(respBody), Body: respBody}
			}
			return printRaw(cmd, env, respBody)
		},
	}

	f := cmd.Flags()
	f.StringVarP(&method, "method", "X", http.MethodGet, "метод HTTP")
	f.StringArrayVarP(&fields, "field", "f", nil,
		"поле тела ключ=значение; числа, true/false и null распознаются (можно повторять)")
	f.StringArrayVarP(&rawFields, "raw-field", "F", nil, "поле тела строкой без разбора типа")
	f.StringArrayVarP(&headers, "header", "H", nil, "дополнительный заголовок вида Имя: значение")
	f.StringVar(&input, "input", "", "файл с телом запроса (- для stdin)")
	f.BoolVar(&listOps, "list", false, "перечислить операции контракта")
	f.BoolVar(&allowUnkn, "allow-unknown", false,
		"не сверять путь с вшитым контрактом (если API новее этого CLI)")
	f.BoolVarP(&includeHdr, "include", "i", false, "печатать статус и заголовки ответа")
	return cmd
}

// normalizePath приводит путь к виду контракта: со слешем в начале и без
// префикса /v1, который пользователь может скопировать из документации.
func normalizePath(p string) string {
	p = strings.TrimSpace(p)
	if i := strings.Index(p, "://"); i >= 0 {
		if j := strings.Index(p[i+3:], "/"); j >= 0 {
			p = p[i+3+j:]
		}
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return strings.TrimSuffix(strings.TrimPrefix(p, "/v1"), "/")
}

func unknownPathError(method, path string) error {
	var b strings.Builder
	fmt.Fprintf(&b, "в контракте нет %s %s", method, path)
	if near := contract.Suggest(path, 5); len(near) > 0 {
		b.WriteString("\nПохожие адреса:")
		for _, op := range near {
			fmt.Fprintf(&b, "\n  %-6s %s", op.Method, op.Path)
		}
	}
	b.WriteString("\nПолный список: tatnet api --list")
	b.WriteString("\nЕсли API новее этого CLI — добавьте --allow-unknown")
	return fmt.Errorf("%s", b.String())
}

func listOperations(cmd *cobra.Command, env *Env, pattern string) error {
	all, err := contract.Operations()
	if err != nil {
		return err
	}
	pattern = strings.ToLower(pattern)
	rows := make([]any, 0, len(all))
	for _, op := range all {
		if pattern != "" && !strings.Contains(strings.ToLower(op.Tag+" "+op.Path+" "+op.ID), pattern) {
			continue
		}
		rows = append(rows, map[string]any{
			"method": op.Method, "path": op.Path, "tag": op.Tag,
			"id": op.ID, "summary": op.Summary,
		})
	}
	return env.Printer.List(rows, []output.Column{
		output.Col("метод", "method"),
		output.Col("путь", "path"),
		output.Col("раздел", "tag"),
		output.Col("описание", "summary"),
		output.WideCol("операция", "id"),
	})
}

// buildBody собирает тело запроса из -f/-F или файла.
func buildBody(cmd *cobra.Command, fields, rawFields []string, input string) ([]byte, error) {
	if input != "" {
		if len(fields) > 0 || len(rawFields) > 0 {
			return nil, fmt.Errorf("--input нельзя сочетать с -f/-F")
		}
		data, err := readFileOrStdin(input)
		if err != nil {
			return nil, fmt.Errorf("--input: %w", err)
		}
		if !json.Valid(data) {
			return nil, fmt.Errorf("--input: содержимое не является корректным JSON")
		}
		return data, nil
	}
	if len(fields) == 0 && len(rawFields) == 0 {
		return nil, nil
	}
	obj := map[string]any{}
	for _, kv := range rawFields {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			return nil, fmt.Errorf("-F %q: ожидается ключ=значение", kv)
		}
		obj[k] = v
	}
	for _, kv := range fields {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			return nil, fmt.Errorf("-f %q: ожидается ключ=значение", kv)
		}
		obj[k] = typedValue(v)
	}
	return json.Marshal(obj)
}

// typedValue распознаёт числа, булевы значения и null — иначе каждое поле
// уезжало бы строкой и сервер отвечал бы 422 на числовой параметр.
func typedValue(v string) any {
	switch v {
	case "true":
		return true
	case "false":
		return false
	case "null":
		return nil
	}
	if n, err := strconv.ParseInt(v, 10, 64); err == nil {
		return n
	}
	if f, err := strconv.ParseFloat(v, 64); err == nil {
		return f
	}
	if strings.HasPrefix(v, "[") || strings.HasPrefix(v, "{") {
		var parsed any
		if json.Unmarshal([]byte(v), &parsed) == nil {
			return parsed
		}
	}
	return v
}

// rawRequest — запрос мимо типизированного клиента, для произвольного адреса.
func (e *Env) rawRequest(cmd *cobra.Command, method, path string, body []byte, headers []string) (int, http.Header, []byte, error) {
	if strings.TrimSpace(e.APIKey) == "" {
		return 0, nil, nil, ErrNoAPIKey{Profile: e.ProfileName}
	}
	url := strings.TrimSuffix(e.BaseURL, "/") + path

	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(cmd.Context(), method, url, reader)
	if err != nil {
		return 0, nil, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+e.APIKey)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, h := range headers {
		k, v, ok := strings.Cut(h, ":")
		if !ok {
			return 0, nil, nil, fmt.Errorf("-H %q: ожидается «Имя: значение»", h)
		}
		req.Header.Set(strings.TrimSpace(k), strings.TrimSpace(v))
	}

	client := &http.Client{Timeout: e.Timeout}
	if e.Debug {
		client.Transport = debugTransport{next: http.DefaultTransport}
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, nil, explainTransport(err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, resp.Header, nil, err
	}
	return resp.StatusCode, resp.Header, data, nil
}

// printRaw печатает тело ответа: JSON — с отступами, прочее — как есть.
func printRaw(cmd *cobra.Command, env *Env, body []byte) error {
	if len(body) == 0 {
		return env.Printer.Message("Готово, тела ответа нет.")
	}
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		_, err := cmd.OutOrStdout().Write(append(bytes.TrimRight(body, "\n"), '\n'))
		return err
	}
	return env.Printer.Raw(v)
}
