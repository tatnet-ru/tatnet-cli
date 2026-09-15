package cli

import (
	"context"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/tatnet-ru/tatnet-go/tatnet"

	"github.com/tatnet-ru/tatnet-cli/internal/output"
)

var valkeyColumns = []output.Column{
	output.Col("имя", "name"),
	output.Col("версия", "valkey_version"),
	output.Col("топология", "topology"),
	output.Col("узлов", "node_count"),
	output.Col("статус", "status"),
	output.Col("хост", "endpoint_host"),
	output.Col("порт", "endpoint_port"),
	output.Col("память", "maxmemory_mb"),
	output.WideCol("id", "id"),
	output.WideCol("вытеснение", "eviction_policy"),
	output.WideCol("persistence", "persistence"),
	output.WideCol("в месяц", "monthly_cost"),
	output.WideCol("подробности", "status_detail"),
}

func newValkeyCommand(env *Env) *cobra.Command {
	cmd := &cobra.Command{Use: "valkey", Short: "Управляемый Valkey (Redis-совместимый кэш)"}
	cmd.AddCommand(
		valkeyListCommand(env),
		valkeyGetCommand(env),
		valkeyCreateCommand(env),
		valkeyDeleteCommand(env),
		valkeyCaCommand(env),
		valkeyParamsCommand(env),
		valkeyAllowlistCommand(env),
		valkeyResetPasswordCommand(env),
		valkeyNodeCommand(env),
		valkeyRegionsCommand(env),
	)
	return cmd
}

func (e *Env) listValkeyClusters(ctx context.Context, project string) ([]any, error) {
	c, err := e.Client()
	if err != nil {
		return nil, err
	}
	return paginate(ctx, func(ctx context.Context, offset, size int) (any, error) {
		return call(c.ValkeyListClustersWithResponse(ctx, project, &tatnet.ValkeyListClustersParams{
			Limit: &size, Offset: &offset,
		}))
	}, 0, 0)
}

func (e *Env) valkeyTarget(ctx context.Context, ref string) (*tatnet.ClientWithResponses, string, string, error) {
	c, err := e.Client()
	if err != nil {
		return nil, "", "", err
	}
	project, err := e.RequireProject(ctx)
	if err != nil {
		return nil, "", "", err
	}
	id, err := resolveRef(ctx, "кластер Valkey", ref, func(ctx context.Context) ([]any, error) {
		return e.listValkeyClusters(ctx, project)
	}, "name")
	if err != nil {
		return nil, "", "", err
	}
	return c, project, id, nil
}

func valkeyListCommand(env *Env) *cobra.Command {
	return &cobra.Command{
		Use:         "list",
		Short:       "Кластеры проекта",
		Annotations: ops("valkey_list_clusters"),
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			project, err := env.RequireProject(cmd.Context())
			if err != nil {
				return err
			}
			all, err := env.listValkeyClusters(cmd.Context(), project)
			if err != nil {
				return err
			}
			return env.Printer.List(all, valkeyColumns)
		},
	}
}

func valkeyGetCommand(env *Env) *cobra.Command {
	return &cobra.Command{
		Use:         "get <кластер>",
		Short:       "Один кластер",
		Annotations: ops("valkey_get_cluster"),
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, project, id, err := env.valkeyTarget(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			v, err := call(c.ValkeyGetClusterWithResponse(cmd.Context(), project, id))
			if err != nil {
				return err
			}
			if env.Printer.Format == output.JSON || env.Printer.Format == output.YAML {
				return env.Printer.Raw(v)
			}
			if err := env.Printer.Object(v, append(valkeyColumns,
				output.Col("dns", "endpoint_dns"),
				output.Col("пользователь", "user"),
				output.Col("maxclients", "maxclients"),
				output.Col("список ip", "ip_allowlist"),
				output.Col("узлы устарели", "nodes_outdated"),
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
				output.Col("роль", "server_role"),
				output.Col("фаза", "phase"),
				output.Col("поколение", "generation"),
				output.Col("образ устарел", "image_outdated"),
				output.Col("смещение", "repl_offset"),
			})
		},
	}
}

func valkeyCreateCommand(env *Env) *cobra.Command {
	var (
		name, plan, version, topology string
		region, vpc                   string
		eviction, persistence         string
		allowlist                     []string
	)
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Создать кластер",
		Long: "Создаёт кластер Valkey.\n\n" +
			"⚠ У Valkey нет аналога pg_hba: список разрешённых адресов —\n" +
			"ЕДИНСТВЕННЫЙ сетевой фильтр перед портом.",
		Annotations: ops("valkey_create_cluster"),
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
			v, err := call(c.ValkeyCreateClusterWithResponse(cmd.Context(), project, tatnet.CreateValkeyClusterRequest{
				Name:           name,
				PlanId:         plan,
				ValkeyVersion:  optStr(cmd, "version", version),
				Topology:       optStr(cmd, "topology", topology),
				ClusterId:      optStr(cmd, "region", region),
				VpcId:          optStr(cmd, "vpc", vpc),
				EvictionPolicy: optStr(cmd, "eviction", eviction),
				Persistence:    optStr(cmd, "persistence", persistence),
				IpAllowlist:    optStrSlice(cmd, "allow", allowlist),
			}))
			if err != nil {
				return err
			}
			if pw := output.Value(v, "password"); pw != "-" {
				fmt.Fprintf(cmd.ErrOrStderr(), "Пароль показывается один раз: %s\n\n", pw)
			}
			return env.Printer.Object(v, valkeyColumns)
		},
	}
	f := cmd.Flags()
	f.StringVar(&name, "name", "", "имя кластера (обязательно)")
	f.StringVar(&plan, "plan", "", "id тарифа (обязательно; см. tatnet valkey regions)")
	f.StringVar(&version, "version", "", "версия Valkey")
	f.StringVar(&topology, "topology", "", "топология: single или ha")
	f.StringVar(&region, "region", "", "id региона")
	f.StringVar(&vpc, "vpc", "", "id приватной сети")
	f.StringVar(&eviction, "eviction", "", "политика вытеснения")
	f.StringVar(&persistence, "persistence", "", "режим сохранения на диск")
	f.StringSliceVar(&allowlist, "allow", nil, "разрешённые CIDR (можно повторять)")
	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("plan")
	return cmd
}

func valkeyDeleteCommand(env *Env) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:         "delete <кластер>",
		Short:       "Удалить кластер",
		Annotations: ops("valkey_delete_cluster"),
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, project, id, err := env.valkeyTarget(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if err := confirm(cmd, yes, "Удалить кластер %s вместе с данными?", args[0]); err != nil {
				return err
			}
			if err := callOK(c.ValkeyDeleteClusterWithResponse(cmd.Context(), project, id)); err != nil {
				return err
			}
			return env.Printer.Message("Кластер %s удаляется.", args[0])
		},
	}
	addYes(cmd, &yes)
	return cmd
}

func valkeyCaCommand(env *Env) *cobra.Command {
	var file string
	cmd := &cobra.Command{
		Use:   "ca <кластер>",
		Short: "Сертификат CA кластера",
		Long: "Valkey принимает только TLS — подключаться нужно по rediss://\n" +
			"с этим сертификатом.",
		Annotations: ops("valkey_get_cluster_ca"),
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, project, id, err := env.valkeyTarget(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			v, err := call(c.ValkeyGetClusterCaWithResponse(cmd.Context(), project, id))
			if err != nil {
				return err
			}
			return writeCA(cmd, env, v, file, args[0])
		},
	}
	cmd.Flags().StringVar(&file, "out", "", "записать PEM в файл")
	return cmd
}

func valkeyParamsCommand(env *Env) *cobra.Command {
	var eviction, persistence string
	cmd := &cobra.Command{
		Use:   "params <кластер>",
		Short: "Изменить параметры кэша",
		Long: "Параметры доводятся до живого сервера без перезапуска\n" +
			"(CONFIG SET + REWRITE).",
		Annotations: ops("valkey_update_params"),
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, project, id, err := env.valkeyTarget(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if !cmd.Flags().Changed("eviction") && !cmd.Flags().Changed("persistence") {
				return fmt.Errorf("укажите --eviction и/или --persistence")
			}
			v, err := call(c.ValkeyUpdateParamsWithResponse(cmd.Context(), project, id,
				tatnet.UpdateValkeyParamsRequest{
					EvictionPolicy: optStr(cmd, "eviction", eviction),
					Persistence:    optStr(cmd, "persistence", persistence),
				}))
			if err != nil {
				return err
			}
			return env.Printer.Object(v, valkeyColumns)
		},
	}
	cmd.Flags().StringVar(&eviction, "eviction", "", "политика вытеснения")
	cmd.Flags().StringVar(&persistence, "persistence", "", "режим сохранения на диск")
	return cmd
}

func valkeyAllowlistCommand(env *Env) *cobra.Command {
	return &cobra.Command{
		Use:   "allowlist <кластер> <cidr>...",
		Short: "Задать список разрешённых адресов",
		Long: "Передаёт список целиком: не указанные адреса теряют доступ.\n" +
			"У Valkey это единственный сетевой фильтр.",
		Annotations: ops("valkey_update_allowlist"),
		Args:        cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, project, id, err := env.valkeyTarget(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			v, err := call(c.ValkeyUpdateAllowlistWithResponse(cmd.Context(), project, id,
				tatnet.UpdateValkeyAllowlistRequest{IpAllowlist: args[1:]}))
			if err != nil {
				return err
			}
			return env.Printer.Object(v, valkeyColumns)
		},
	}
}

func valkeyResetPasswordCommand(env *Env) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "reset-password <кластер>",
		Short: "Сбросить пароль",
		Long: "Новый пароль показывается один раз. Старый перестаёт работать\n" +
			"сразу, как только регион его применит.",
		Annotations: ops("valkey_reset_password"),
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, project, id, err := env.valkeyTarget(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if err := confirm(cmd, yes, "Сбросить пароль кластера %s?", args[0]); err != nil {
				return err
			}
			v, err := call(c.ValkeyResetPasswordWithResponse(cmd.Context(), project, id))
			if err != nil {
				return err
			}
			return env.Printer.Object(v, []output.Column{
				output.Col("пароль", "password"),
				output.Col("применён", "applied"),
			})
		},
	}
	addYes(cmd, &yes)
	return cmd
}

func valkeyNodeCommand(env *Env) *cobra.Command {
	cmd := &cobra.Command{Use: "node", Aliases: []string{"nodes"}, Short: "Узлы кластера"}

	var yes bool
	replace := &cobra.Command{
		Use:   "replace <кластер> <номер>",
		Short: "Заменить узел на новое поколение",
		Long: "Поднимает узел того же номера на свежем образе и выводит старый.\n" +
			"На стыке в кластере недолго два мастера — это штатный ход замены.",
		Annotations: ops("valkey_replace_node"),
		Args:        cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, project, id, err := env.valkeyTarget(cmd.Context(), args[0])
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
			v, err := call(c.ValkeyReplaceNodeWithResponse(cmd.Context(), project, id, ordinal))
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
		Annotations: ops("valkey_replace_outdated_nodes"),
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, project, id, err := env.valkeyTarget(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if err := confirm(cmd, yesAll, "Заменить все устаревшие узлы кластера %s?", args[0]); err != nil {
				return err
			}
			v, err := call(c.ValkeyReplaceOutdatedNodesWithResponse(cmd.Context(), project, id))
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

func valkeyRegionsCommand(env *Env) *cobra.Command {
	return &cobra.Command{
		Use:         "regions",
		Short:       "Регионы, версии и тарифы",
		Annotations: ops("valkey_list_regions"),
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := env.Client()
			if err != nil {
				return err
			}
			v, err := call(c.ValkeyListRegionsWithResponse(cmd.Context()))
			if err != nil {
				return err
			}
			if env.Printer.Format == output.JSON || env.Printer.Format == output.YAML {
				return env.Printer.Raw(v)
			}
			fmt.Fprintf(cmd.OutOrStdout(),
				"версии       %s\nпо умолчанию %s\nтопологии    %s\nвытеснение   %s\nсохранение   %s\n\nрегионы\n",
				output.Value(v, "valkey_versions"), output.Value(v, "default_valkey_version"),
				output.Value(v, "topologies"), output.Value(v, "eviction_policies"),
				output.Value(v, "persistence_modes"))
			if err := env.Printer.List(fieldItems(v, "regions"), []output.Column{
				output.Col("имя", "name"),
				output.Col("id", "id"),
				output.Col("по умолчанию", "is_default"),
				output.Col("версии", "valkey_versions"),
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
				output.Col("память ВМ", "mem"),
				output.Col("maxmemory", "maxmemory_mb"),
				output.Col("в месяц", "price_per_month"),
			})
		},
	}
}
