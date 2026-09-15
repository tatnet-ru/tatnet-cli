package cli

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tatnet-ru/tatnet-go/tatnet"

	"github.com/tatnet-ru/tatnet-cli/internal/output"
)

var pgColumns = []output.Column{
	output.Col("имя", "name"),
	output.Col("версия", "pg_version"),
	output.Col("топология", "topology"),
	output.Col("узлов", "node_count"),
	output.Col("статус", "status"),
	output.Col("хост", "host"),
	output.Col("порт", "port"),
	output.WideCol("id", "id"),
	output.WideCol("тариф", "plan.name"),
	output.WideCol("в месяц", "monthly_cost"),
	output.WideCol("подробности", "status_detail"),
}

func newPGCommand(env *Env) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "pg",
		Aliases: []string{"postgres"},
		Short:   "Управляемый PostgreSQL",
	}
	cmd.AddCommand(
		pgListCommand(env),
		pgGetCommand(env),
		pgCreateCommand(env),
		pgDeleteCommand(env),
		pgRestartCommand(env),
		pgCaCommand(env),
		pgBackupCommand(env),
		pgParametersCommand(env),
		pgPlanCommand(env),
		pgTopologyCommand(env),
		pgAllowlistCommand(env),
		pgResetPasswordCommand(env),
		pgNodeCommand(env),
		pgRegionsCommand(env),
	)
	return cmd
}

func (e *Env) listPGClusters(ctx context.Context, project string) ([]any, error) {
	c, err := e.Client()
	if err != nil {
		return nil, err
	}
	return paginate(ctx, func(ctx context.Context, offset, size int) (any, error) {
		return call(c.PostgresListClustersWithResponse(ctx, project, &tatnet.PostgresListClustersParams{
			Limit: &size, Offset: &offset,
		}))
	}, 0, 0)
}

func (e *Env) pgTarget(ctx context.Context, ref string) (*tatnet.ClientWithResponses, string, string, error) {
	c, err := e.Client()
	if err != nil {
		return nil, "", "", err
	}
	project, err := e.RequireProject(ctx)
	if err != nil {
		return nil, "", "", err
	}
	id, err := resolveRef(ctx, "кластер PostgreSQL", ref, func(ctx context.Context) ([]any, error) {
		return e.listPGClusters(ctx, project)
	}, "name")
	if err != nil {
		return nil, "", "", err
	}
	return c, project, id, nil
}

func pgListCommand(env *Env) *cobra.Command {
	return &cobra.Command{
		Use:         "list",
		Short:       "Кластеры проекта",
		Annotations: ops("postgres_list_clusters"),
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			project, err := env.RequireProject(cmd.Context())
			if err != nil {
				return err
			}
			all, err := env.listPGClusters(cmd.Context(), project)
			if err != nil {
				return err
			}
			return env.Printer.List(all, pgColumns)
		},
	}
}

func pgGetCommand(env *Env) *cobra.Command {
	return &cobra.Command{
		Use:         "get <кластер>",
		Short:       "Один кластер",
		Annotations: ops("postgres_get_cluster"),
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, project, id, err := env.pgTarget(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			v, err := call(c.PostgresGetClusterWithResponse(cmd.Context(), project, id))
			if err != nil {
				return err
			}
			if env.Printer.Format == output.JSON || env.Printer.Format == output.YAML {
				return env.Printer.Raw(v)
			}
			if err := env.Printer.Object(v, append(pgColumns,
				output.Col("dns", "endpoint_dns"),
				output.Col("read-хост", "read_host"),
				output.Col("список ip", "ip_allowlist"),
				output.Col("параметры применены", "parameters_applied"),
				output.Col("нужен перезапуск", "pending_restart"),
				output.Col("узлы устарели", "nodes_outdated"),
				output.Col("бэкапы", "backups_enabled"),
				output.Col("wal заархивирован", "wal_archived_at"),
			)); err != nil {
				return err
			}
			nodes := fieldItems(v, "nodes")
			if len(nodes) == 0 {
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), "\nузлы")
			return env.Printer.List(nodes, []output.Column{
				output.Col("№", "ordinal"),
				output.Col("роль", "patroni_role"),
				output.Col("фаза", "phase"),
				output.Col("поколение", "generation"),
				output.Col("образ устарел", "image_outdated"),
				output.Col("отставание", "replication_lag_bytes"),
			})
		},
	}
}

func pgCreateCommand(env *Env) *cobra.Command {
	var (
		name, plan, version, topology  string
		cluster, vpc, database, dbUser string
		allowlist                      []string
	)
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Создать кластер",
		Annotations: ops("postgres_create_cluster"),
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := env.Client()
			if err != nil {
				return err
			}
			project, err := env.RequireProject(cmd.Context())
			if err != nil {
				return err
			}
			v, err := call(c.PostgresCreateClusterWithResponse(cmd.Context(), project, tatnet.CreatePgClusterRequest{
				Name:        name,
				PlanId:      plan,
				PgVersion:   optStr(cmd, "version", version),
				Topology:    optStr(cmd, "topology", topology),
				ClusterId:   optStr(cmd, "region", cluster),
				VpcId:       optStr(cmd, "vpc", vpc),
				Database:    optStr(cmd, "database", database),
				DbUser:      optStr(cmd, "db-user", dbUser),
				IpAllowlist: optStrSlice(cmd, "allow", allowlist),
			}))
			if err != nil {
				return err
			}
			if pw := output.Value(v, "role_password"); pw != "-" {
				fmt.Fprintf(cmd.ErrOrStderr(),
					"Пароль владельца базы показывается один раз: %s\n\n", pw)
			}
			return env.Printer.Object(v, pgColumns)
		},
	}
	f := cmd.Flags()
	f.StringVar(&name, "name", "", "имя кластера (обязательно)")
	f.StringVar(&plan, "plan", "", "id тарифа (обязательно; см. tatnet pg regions)")
	f.StringVar(&version, "version", "", "мажорная версия PostgreSQL")
	f.StringVar(&topology, "topology", "", "топология: single или ha")
	f.StringVar(&cluster, "region", "", "id региона")
	f.StringVar(&vpc, "vpc", "", "id приватной сети (кластер получит адрес в ней)")
	f.StringVar(&database, "database", "", "имя базы")
	f.StringVar(&dbUser, "db-user", "", "имя владельца базы")
	f.StringSliceVar(&allowlist, "allow", nil, "разрешённые CIDR (можно повторять)")
	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("plan")
	return cmd
}

func pgDeleteCommand(env *Env) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:         "delete <кластер>",
		Short:       "Удалить кластер",
		Annotations: ops("postgres_delete_cluster"),
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, project, id, err := env.pgTarget(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if err := confirm(cmd, yes, "Удалить кластер %s вместе с данными?", args[0]); err != nil {
				return err
			}
			if err := callOK(c.PostgresDeleteClusterWithResponse(cmd.Context(), project, id)); err != nil {
				return err
			}
			return env.Printer.Message("Кластер %s удаляется.", args[0])
		},
	}
	addYes(cmd, &yes)
	return cmd
}

func pgRestartCommand(env *Env) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "restart <кластер>",
		Short: "Перезапустить кластер",
		Long: "Запрашивает перезапуск: реплики первыми, лидер последним.\n" +
			"Нужен после смены параметров уровня postmaster.",
		Annotations: ops("postgres_restart_cluster"),
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, project, id, err := env.pgTarget(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if err := confirm(cmd, yes, "Перезапустить кластер %s? Соединения будут разорваны.", args[0]); err != nil {
				return err
			}
			v, err := call(c.PostgresRestartClusterWithResponse(cmd.Context(), project, id))
			if err != nil {
				return err
			}
			return env.Printer.Raw(v)
		},
	}
	addYes(cmd, &yes)
	return cmd
}

func pgCaCommand(env *Env) *cobra.Command {
	var file string
	cmd := &cobra.Command{
		Use:   "ca <кластер>",
		Short: "Сертификат CA кластера",
		Long: "Выдаёт корневой сертификат кластера — он нужен для подключения\n" +
			"с проверкой сервера (sslmode=verify-full).",
		Annotations: ops("postgres_get_cluster_ca"),
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, project, id, err := env.pgTarget(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			v, err := call(c.PostgresGetClusterCaWithResponse(cmd.Context(), project, id))
			if err != nil {
				return err
			}
			return writeCA(cmd, env, v, file, args[0])
		},
	}
	cmd.Flags().StringVar(&file, "out", "", "записать PEM в файл")
	return cmd
}

// writeCA — общий вывод CA для pg и valkey.
func writeCA(cmd *cobra.Command, env *Env, v any, file, ref string) error {
	if env.Printer.Format == output.JSON || env.Printer.Format == output.YAML {
		return env.Printer.Raw(v)
	}
	pem := output.Value(v, "ca_pem")
	if pem == "-" || strings.TrimSpace(pem) == "" {
		if output.Value(v, "ready") == "нет" {
			return fmt.Errorf("сертификат кластера %s ещё не готов — регион его не выпустил", ref)
		}
		return fmt.Errorf("кластер %s не вернул сертификат", ref)
	}
	if file == "" {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), strings.TrimRight(pem, "\n"))
		return err
	}
	if err := os.WriteFile(file, []byte(pem), 0o644); err != nil {
		return err
	}
	return env.Printer.Message("Сертификат записан в %s", file)
}

func pgBackupCommand(env *Env) *cobra.Command {
	cmd := &cobra.Command{Use: "backup", Aliases: []string{"backups"}, Short: "Бэкапы и PITR"}

	backupColumns := []output.Column{
		output.Col("метка", "label"),
		output.Col("вид", "kind"),
		output.Col("тип", "backup_type"),
		output.Col("статус", "status"),
		output.Col("размер", "size_bytes"),
		output.Col("завершён", "completed_at"),
		output.WideCol("id", "id"),
		output.WideCol("истекает", "expires_at"),
		output.WideCol("ошибка", "error"),
	}

	list := &cobra.Command{
		Use:         "list <кластер>",
		Short:       "Бэкапы кластера",
		Annotations: ops("postgres_list_backups"),
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, project, id, err := env.pgTarget(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			v, err := call(c.PostgresListBackupsWithResponse(cmd.Context(), project, id))
			if err != nil {
				return err
			}
			if env.Printer.Format == output.JSON || env.Printer.Format == output.YAML {
				return env.Printer.Raw(v)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "окно PITR      %s\nхранение, дней %s\nв репозитории  %s\n\n",
				output.Value(v, "pitr_window"), output.Value(v, "retention_days"), output.Value(v, "repo_bytes"))
			return env.Printer.List(fieldItems(v, "backups"), backupColumns)
		},
	}

	create := &cobra.Command{
		Use:         "create <кластер> <метка>",
		Short:       "Снять полный бэкап",
		Annotations: ops("postgres_create_backup"),
		Args:        cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, project, id, err := env.pgTarget(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			v, err := call(c.PostgresCreateBackupWithResponse(cmd.Context(), project, id,
				tatnet.CreatePgBackupRequest{Label: args[1]}))
			if err != nil {
				return err
			}
			return env.Printer.Object(v, backupColumns)
		},
	}

	var yes bool
	del := &cobra.Command{
		Use:         "delete <кластер> <бэкап-id>",
		Short:       "Удалить бэкап",
		Annotations: ops("postgres_delete_backup"),
		Args:        cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, project, id, err := env.pgTarget(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if err := confirm(cmd, yes, "Удалить бэкап %s? Окно восстановления сократится.", args[1]); err != nil {
				return err
			}
			if err := callOK(c.PostgresDeleteBackupWithResponse(cmd.Context(), project, id, args[1])); err != nil {
				return err
			}
			return env.Printer.Message("Бэкап %s удаляется.", args[1])
		},
	}
	addYes(del, &yes)

	cmd.AddCommand(list, create, del)
	return cmd
}

func pgParametersCommand(env *Env) *cobra.Command {
	cmd := &cobra.Command{Use: "parameters", Aliases: []string{"params"}, Short: "Параметры postgresql.conf"}

	get := &cobra.Command{
		Use:   "get <кластер>",
		Short: "Заданные и применённые параметры",
		Long: "Показывает намерение (что задано) рядом с наблюдением (что регион\n" +
			"реально применил). Расходящиеся строки помечены.",
		Annotations: ops("postgres_get_parameters"),
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, project, id, err := env.pgTarget(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			v, err := call(c.PostgresGetParametersWithResponse(cmd.Context(), project, id))
			if err != nil {
				return err
			}
			if env.Printer.Format == output.JSON || env.Printer.Format == output.YAML {
				return env.Printer.Raw(v)
			}
			m, _ := v.(map[string]any)
			intent, _ := m["parameters"].(map[string]any)
			applied, _ := m["applied_parameters"].(map[string]any)

			keys := map[string]bool{}
			for k := range intent {
				keys[k] = true
			}
			for k := range applied {
				keys[k] = true
			}
			names := make([]string, 0, len(keys))
			for k := range keys {
				names = append(names, k)
			}
			sort.Strings(names)

			rows := make([]any, 0, len(names))
			for _, n := range names {
				state := "применён"
				switch {
				case intent[n] == nil:
					state = "снимается"
				case applied[n] == nil:
					state = "ждёт"
				case output.Stringify(intent[n]) != output.Stringify(applied[n]):
					state = "расходится"
				}
				rows = append(rows, map[string]any{
					"name": n, "value": intent[n], "applied": applied[n], "state": state,
				})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "всё применено   %s\nждёт перезапуска %s\n\n",
				output.Value(v, "applied"), output.Value(v, "pending_restart"))
			return env.Printer.List(rows, []output.Column{
				output.Col("параметр", "name"),
				output.Col("задано", "value"),
				output.Col("применено", "applied"),
				output.Col("состояние", "state"),
			})
		},
	}

	var params []string
	var reset bool
	set := &cobra.Command{
		Use:   "set <кластер> параметр=значение...",
		Short: "Задать параметры",
		Long: "Передаёт набор переопределений целиком: параметры, которых нет\n" +
			"в вызове, снимаются и возвращаются к значению по умолчанию.\n" +
			"Поэтому сначала посмотрите `tatnet pg parameters get`.",
		Annotations: ops("postgres_update_parameters"),
		Args:        cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, project, id, err := env.pgTarget(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			pairs := append([]string{}, args[1:]...)
			pairs = append(pairs, params...)
			if len(pairs) == 0 && !reset {
				return fmt.Errorf("не заданы параметры; чтобы снять все, добавьте --reset")
			}
			values := map[string]string{}
			for _, p := range pairs {
				k, v, ok := strings.Cut(p, "=")
				if !ok {
					return fmt.Errorf("%q: ожидается параметр=значение", p)
				}
				values[strings.TrimSpace(k)] = strings.TrimSpace(v)
			}
			v, err := call(c.PostgresUpdateParametersWithResponse(cmd.Context(), project, id,
				tatnet.UpdatePgParametersRequest{Parameters: &values}))
			if err != nil {
				return err
			}
			return env.Printer.Message("Задано параметров: %d. Применение проверяйте в `tatnet pg parameters get`.%s",
				len(values), restartHint(v))
		},
	}
	set.Flags().StringArrayVar(&params, "set", nil, "параметр=значение (можно повторять)")
	set.Flags().BoolVar(&reset, "reset", false, "снять все переопределения")

	cmd.AddCommand(get, set)
	return cmd
}

func restartHint(v any) string {
	if output.Value(v, "pending_restart") == "да" {
		return "\nЧасть параметров требует перезапуска: tatnet pg restart."
	}
	return ""
}

func pgPlanCommand(env *Env) *cobra.Command {
	return &cobra.Command{
		Use:   "plan <кластер> <тариф-id>",
		Short: "Сменить тариф",
		Long: "Тариф меняется только вверх: память, vCPU и диск должны быть не\n" +
			"меньше текущих. Узлы ресайзятся по одному, лидер последним.",
		Annotations: ops("postgres_change_plan"),
		Args:        cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, project, id, err := env.pgTarget(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			v, err := call(c.PostgresChangePlanWithResponse(cmd.Context(), project, id,
				tatnet.ChangePgPlanRequest{PlanId: args[1]}))
			if err != nil {
				return err
			}
			return env.Printer.Object(v, pgColumns)
		},
	}
}

func pgTopologyCommand(env *Env) *cobra.Command {
	return &cobra.Command{
		Use:         "topology <кластер> <single|ha>",
		Short:       "Сменить топологию",
		Annotations: ops("postgres_change_topology"),
		Args:        cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, project, id, err := env.pgTarget(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			v, err := call(c.PostgresChangeTopologyWithResponse(cmd.Context(), project, id,
				tatnet.ChangePgTopologyRequest{Topology: args[1]}))
			if err != nil {
				return err
			}
			return env.Printer.Object(v, pgColumns)
		},
	}
}

func pgAllowlistCommand(env *Env) *cobra.Command {
	return &cobra.Command{
		Use:   "allowlist <кластер> <cidr>...",
		Short: "Задать список разрешённых адресов",
		Long: "Передаёт список целиком: адреса, не указанные в вызове, теряют\n" +
			"доступ. Для публичного кластера это единственный сетевой фильтр.",
		Annotations: ops("postgres_update_allowlist"),
		Args:        cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, project, id, err := env.pgTarget(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			v, err := call(c.PostgresUpdateAllowlistWithResponse(cmd.Context(), project, id,
				tatnet.UpdatePgAllowlistRequest{IpAllowlist: args[1:]}))
			if err != nil {
				return err
			}
			return env.Printer.Object(v, pgColumns)
		},
	}
}

func pgResetPasswordCommand(env *Env) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "reset-password <кластер> <роль>",
		Short: "Сбросить пароль роли",
		Long: "Новый пароль показывается один раз, в ответе. Старый перестаёт\n" +
			"работать сразу — приложения с ним получат отказ.",
		Annotations: ops("postgres_reset_role_password"),
		Args:        cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, project, id, err := env.pgTarget(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if err := confirm(cmd, yes, "Сбросить пароль роли %s? Старый перестанет работать немедленно.", args[1]); err != nil {
				return err
			}
			v, err := call(c.PostgresResetRolePasswordWithResponse(cmd.Context(), project, id, args[1]))
			if err != nil {
				return err
			}
			return env.Printer.Raw(v)
		},
	}
	addYes(cmd, &yes)
	return cmd
}

func pgNodeCommand(env *Env) *cobra.Command {
	cmd := &cobra.Command{Use: "node", Aliases: []string{"nodes"}, Short: "Узлы кластера"}

	var yes bool
	replace := &cobra.Command{
		Use:   "replace <кластер> <номер>",
		Short: "Заменить узел на новое поколение",
		Long: "Поднимает узел того же порядкового номера на свежем образе,\n" +
			"вводит его в кластер и выводит старый. Лидер переключается сам.",
		Annotations: ops("postgres_replace_node"),
		Args:        cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, project, id, err := env.pgTarget(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			ordinal, err := strconv.Atoi(args[1])
			if err != nil {
				return fmt.Errorf("номер узла должен быть числом: %w", err)
			}
			if err := confirm(cmd, yes, "Заменить узел %d кластера %s?", ordinal, args[0]); err != nil {
				return err
			}
			v, err := call(c.PostgresReplaceNodeWithResponse(cmd.Context(), project, id, ordinal))
			if err != nil {
				return err
			}
			return env.Printer.Raw(v)
		},
	}
	addYes(replace, &yes)

	var yesAll bool
	replaceAll := &cobra.Command{
		Use:         "replace-outdated <кластер>",
		Short:       "Заменить все узлы на устаревшем образе",
		Annotations: ops("postgres_replace_outdated_nodes"),
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, project, id, err := env.pgTarget(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if err := confirm(cmd, yesAll, "Заменить все устаревшие узлы кластера %s? Узлы пойдут по одному.", args[0]); err != nil {
				return err
			}
			v, err := call(c.PostgresReplaceOutdatedNodesWithResponse(cmd.Context(), project, id))
			if err != nil {
				return err
			}
			return env.Printer.Raw(v)
		},
	}
	addYes(replaceAll, &yesAll)

	cmd.AddCommand(replace, replaceAll)
	return cmd
}

func pgRegionsCommand(env *Env) *cobra.Command {
	return &cobra.Command{
		Use:         "regions",
		Short:       "Регионы, версии и тарифы",
		Annotations: ops("postgres_list_regions"),
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := env.Client()
			if err != nil {
				return err
			}
			v, err := call(c.PostgresListRegionsWithResponse(cmd.Context()))
			if err != nil {
				return err
			}
			if env.Printer.Format == output.JSON || env.Printer.Format == output.YAML {
				return env.Printer.Raw(v)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "версии       %s\nпо умолчанию %s\nтопологии    %s\n\nрегионы\n",
				output.Value(v, "pg_versions"), output.Value(v, "default_pg_version"), output.Value(v, "topologies"))
			if err := env.Printer.List(fieldItems(v, "regions"), []output.Column{
				output.Col("имя", "name"),
				output.Col("id", "cluster_id"),
				output.Col("по умолчанию", "is_default"),
				output.Col("версии", "pg_versions"),
			}); err != nil {
				return err
			}
			plans := fieldItems(v, "plans")
			if len(plans) == 0 {
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), "\nтарифы")
			return env.Printer.List(plans, []output.Column{
				output.Col("имя", "name"),
				output.Col("id", "id"),
				output.Col("vcpu", "vcpu"),
				output.Col("память", "mem"),
				output.Col("диск", "disk_size"),
				output.Col("в месяц", "price_per_month"),
			})
		},
	}
}

// fieldItems достаёт массив из именованного поля ответа. Нужен там, где ответ
// не страничный конверт, а объект со списком внутри.
func fieldItems(v any, name string) []any {
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	arr, _ := m[name].([]any)
	return arr
}
