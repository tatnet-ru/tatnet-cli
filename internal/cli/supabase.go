package cli

import (
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"io"
	"strings"
)

func newSupabaseCommand(env *Env) *cobra.Command {
	group := &cobra.Command{Use: "supabase", Short: "Управляемые базы Supabase"}
	var limit, offset int
	list := &cobra.Command{Use: "list", Short: "Доступные базы без секретов", Args: cobra.NoArgs, Annotations: ops("supabase_list_databases"), RunE: func(cmd *cobra.Command, _ []string) error {
		if limit < 1 || limit > 200 || offset < 0 {
			return fmt.Errorf("неверные параметры страницы")
		}
		return supabaseCall(cmd, env, "GET", fmt.Sprintf("/supabase?limit=%d&offset=%d", limit, offset), nil)
	}}
	list.Flags().IntVar(&limit, "limit", 50, "размер страницы, до 200")
	list.Flags().IntVar(&offset, "offset", 0, "смещение страницы")
	group.AddCommand(list)
	for _, write := range []bool{false, true} {
		name, op, short := "query", "query_database", "Один SQL-запрос в режиме только чтения"
		if write {
			name, op, short = "sql", "execute_sql", "SQL от владельца базы; может удалить данные"
		}
		var query string
		var maxRows, timeoutMS int
		var yes bool
		cmd := &cobra.Command{Use: name + " <database-id>", Short: short, Args: cobra.ExactArgs(1), Annotations: ops("supabase_" + op), RunE: func(cmd *cobra.Command, args []string) error {
			id, err := uuid.Parse(args[0])
			if err != nil {
				return fmt.Errorf("укажите UUID базы")
			}
			sql := query
			if sql == "-" {
				raw, err := io.ReadAll(io.LimitReader(cmd.InOrStdin(), 100001))
				if err != nil {
					return err
				}
				sql = string(raw)
			}
			if strings.TrimSpace(sql) == "" || len(sql) > 100000 || maxRows < 1 || maxRows > 1000 || timeoutMS < 100 || timeoutMS > 30000 {
				return fmt.Errorf("проверьте SQL, --max-rows и --timeout-ms")
			}
			if write {
				if err := confirm(cmd, yes, "Выполнить SQL с правом изменения и удаления данных в базе %s?", id); err != nil {
					return err
				}
			}
			body, _ := json.Marshal(map[string]any{"sql": sql, "timeout_ms": timeoutMS, "max_rows": maxRows})
			suffix := "query"
			if write {
				suffix = "sql"
			}
			return supabaseCall(cmd, env, "POST", "/supabase/"+id.String()+"/"+suffix, body)
		}}
		cmd.Flags().StringVar(&query, "sql", "", "SQL либо - для чтения из stdin")
		cmd.Flags().IntVar(&maxRows, "max-rows", 100, "строк на запрос, до 1000")
		cmd.Flags().IntVar(&timeoutMS, "timeout-ms", 30000, "таймаут SQL, до 30000 мс")
		if write {
			addYes(cmd, &yes)
		}
		group.AddCommand(cmd)
	}
	return group
}

func supabaseCall(cmd *cobra.Command, env *Env, method, path string, body []byte) error {
	status, _, raw, err := env.rawRequest(cmd, method, path, body, nil)
	if err != nil {
		return fmt.Errorf("%w; SQL не повторяйте автоматически: результат может быть неизвестен", err)
	}
	if status < 200 || status >= 300 {
		return &APIError{Status: status, Detail: detailOf(raw), Body: raw}
	}
	var result struct {
		OK *bool `json:"ok"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return err
	}
	if err := printRaw(cmd, env, raw); err != nil {
		return err
	}
	if result.OK != nil && !*result.OK {
		return fmt.Errorf("SQL завершился ошибкой; подробности в ответе")
	}
	return nil
}
