package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/tatnet-ru/tatnet-go/tatnet"

	"github.com/tatnet-ru/tatnet-cli/internal/output"
)

var vmColumns = []output.Column{
	output.Col("имя", "name"),
	output.Col("хост", "hostname"),
	output.Col("статус", "status"),
	output.Col("vcpu", "vcpu"),
	output.Col("память", "mem"),
	output.Col("диск", "disk_size"),
	output.Col("адреса", "ipv4_addresses"),
	output.WideCol("id", "id"),
	output.WideCol("гостевой агент", "guest_agent_state"),
	output.WideCol("проект", "project_id"),
}

func newVMCommand(env *Env) *cobra.Command {
	cmd := &cobra.Command{Use: "vm", Aliases: []string{"vms"}, Short: "Виртуальные машины"}
	cmd.AddCommand(
		vmListCommand(env),
		vmPlansCommand(env),
		vmGetCommand(env),
		vmCreateCommand(env),
		vmDeleteCommand(env),
		vmStartCommand(env),
		vmPowerCommand(env, "stop", "Выключить ВМ", "vms_stop_vm"),
		vmPowerCommand(env, "restart", "Перезагрузить ВМ", "vms_restart_vm"),
		vmBackupCommand(env),
		vmSubscriptionCommand(env),
		vmInterfaceCommand(env),
	)
	cmd.AddCommand(newProjectImagesCommand(env))
	return cmd
}

func vmPlansCommand(env *Env) *cobra.Command {
	var cluster, role string
	cmd := &cobra.Command{
		Use: "plans", Short: "Доступные тарифы ВМ (память в МиБ, цены в рублях)",
		Args:        cobra.NoArgs,
		Annotations: ops("vms_list_plans"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			params := &tatnet.VmsListPlansParams{Role: ptr(tatnet.VmsListPlansParamsRole(role))}
			if cluster != "" {
				id, err := uuid.Parse(cluster)
				if err != nil {
					return fmt.Errorf("--cluster: нужен UUID региона: %w", err)
				}
				params.ClusterId = &id
			}
			c, err := env.Client()
			if err != nil {
				return err
			}
			v, err := call(c.VmsListPlansWithResponse(cmd.Context(), params))
			if err != nil {
				return err
			}
			catalog, ok := v.(map[string]any)
			if !ok {
				return fmt.Errorf("API вернул некорректный каталог тарифов")
			}
			plans, ok := catalog["plans"].([]any)
			if !ok {
				return fmt.Errorf("API вернул каталог без списка plans")
			}
			return env.Printer.List(plans, []output.Column{
				output.Col("id", "id"), output.Col("тариф", "name"),
				output.Col("семейство", "billing_plan_group_name"),
				output.Col("vcpu", "vcpu"), output.Col("память, МиБ", "mem"),
				output.Col("диск, ГиБ", "disk_size"), output.Col("руб/мес", "price_per_month"),
				output.WideCol("руб/час", "price_per_hour"), output.WideCol("теги", "tags"),
			})
		},
	}
	cmd.Flags().StringVar(&cluster, "cluster", "", "UUID региона (фильтр совместимого оборудования)")
	cmd.Flags().StringVar(&role, "role", "vm", "назначение тарифа: vm, k8s, db, lb")
	return cmd
}

// listVMs — все ВМ проекта.
func (e *Env) listVMs(ctx context.Context, project string, limit int) ([]any, error) {
	c, err := e.Client()
	if err != nil {
		return nil, err
	}
	// Плоский список по аккаунту (api#1060); проект — необязательный фильтр.
	var pid *string
	if project != "" {
		pid = &project
	}
	return paginate(ctx, func(ctx context.Context, offset, size int) (any, error) {
		return call(c.VmsListVmsByAccountWithResponse(ctx, &tatnet.VmsListVmsByAccountParams{
			ProjectId: pid, Limit: &size, Offset: &offset,
		}))
	}, limit, 0)
}

func vmListCommand(env *Env) *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "Список ВМ проекта (без -p — по всему аккаунту)",
		Annotations: ops("vms_list_vms_by_account"),
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			project, err := env.optionalProject(cmd.Context())
			if err != nil {
				return err
			}
			all, err := env.listVMs(cmd.Context(), project, limit)
			if err != nil {
				return err
			}
			return env.Printer.List(all, vmColumns)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 0, "не больше стольких записей (0 — все)")
	return cmd
}

func vmGetCommand(env *Env) *cobra.Command {
	return &cobra.Command{
		Use:         "get <вм>",
		Short:       "Одна ВМ",
		Annotations: ops("vms_get_vm"),
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, project, id, err := env.vmTarget(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			v, err := call(c.VmsGetVmWithResponse(cmd.Context(), project, id))
			if err != nil {
				return err
			}
			return env.Printer.Object(v, append(vmColumns,
				output.Col("интерфейсы", "interfaces"),
				output.Col("плавающие адреса", "floating_ips"),
			))
		},
	}
}

func vmCreateCommand(env *Env) *cobra.Command {
	var (
		name, hostname, image, plan, cluster, user string
		cloudInitFile                              string
		disk, months, days                         int
		backupHour, backupDays                     int
		automaticBackups                           bool
		sshKeys, ifaces                            []string
		guestAgent, autoRenew                      bool
	)
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Создать ВМ",
		Long: "Создаёт ВМ.\n\n" +
			"⚠ Идентификаторы образа, тарифа и региона (--image, --plan, --cluster)\n" +
			"публичный API пока не перечисляет — списка образов и тарифов в /v1 нет.\n" +
			"Взять их можно в панели или запросом `tatnet api` к внутренним адресам.",
		Annotations: ops("vms_create_vm"),
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !automaticBackups && (cmd.Flags().Changed("backup-hour-utc") || cmd.Flags().Changed("backup-retention-days")) {
				return fmt.Errorf("для расписания укажите --auto-backups")
			}
			if automaticBackups && (backupHour < 0 || backupHour > 23 || backupDays < 1 || backupDays > 365) {
				return fmt.Errorf("час бэкапа: 0–23 UTC; срок хранения: 1–365 дней")
			}
			c, err := env.Client()
			if err != nil {
				return err
			}
			project, err := env.RequireProject(cmd.Context())
			if err != nil {
				return err
			}
			if hostname == "" {
				hostname = name
			}
			body := tatnet.V1VMCreate{
				Name:              name,
				Hostname:          hostname,
				ImageId:           image,
				VmPlanId:          plan,
				ClusterId:         cluster,
				DefaultUser:       user,
				DiskSizeGb:        optInt(cmd, "disk", disk),
				PeriodMonths:      optInt(cmd, "period-months", months),
				PeriodDays:        optInt(cmd, "period-days", days),
				AutoRenew:         optBool(cmd, "auto-renew", autoRenew),
				InstallGuestAgent: optBool(cmd, "guest-agent", guestAgent),
				SshKeyIds:         optStrSlice(cmd, "ssh-key", sshKeys),
			}
			if automaticBackups {
				body.BackupPolicy = &tatnet.VMBackupPolicyRequest{Enabled: ptr(true), HourUtc: ptr(backupHour), RetentionDays: ptr(backupDays)}
			}
			if cloudInitFile != "" {
				data, err := readFileOrStdin(cloudInitFile)
				if err != nil {
					return fmt.Errorf("--cloud-init: %w", err)
				}
				body.CloudInit = ptr(string(data))
			}
			if len(ifaces) > 0 {
				parsed := make([]tatnet.V1VMInterface, 0, len(ifaces))
				for _, spec := range ifaces {
					iface, err := parseInterface(spec)
					if err != nil {
						return err
					}
					parsed = append(parsed, iface)
				}
				body.Interfaces = &parsed
			}
			v, err := call(c.VmsCreateVmWithResponse(cmd.Context(), project, body))
			if err != nil {
				return err
			}
			return env.Printer.Object(v, vmColumns)
		},
	}
	f := cmd.Flags()
	f.StringVar(&name, "name", "", "имя ВМ (обязательно)")
	f.StringVar(&hostname, "hostname", "", "hostname (по умолчанию — имя)")
	f.StringVar(&image, "image", "", "id образа (обязательно)")
	f.StringVar(&plan, "plan", "", "id тарифа (обязательно)")
	f.StringVar(&cluster, "cluster", "", "id региона (обязательно)")
	f.StringVar(&user, "user", "", "имя пользователя по умолчанию (обязательно)")
	f.IntVar(&disk, "disk", 0, "размер диска, ГБ")
	f.StringSliceVar(&sshKeys, "ssh-key", nil, "id SSH-ключей (можно повторять)")
	f.StringVar(&cloudInitFile, "cloud-init", "", "файл с cloud-init (- для stdin)")
	f.BoolVar(&guestAgent, "guest-agent", false, "ставить qemu-guest-agent")
	f.BoolVar(&automaticBackups, "auto-backups", false, "ежедневные автобэкапы с оплатой занятого хранилища")
	f.IntVar(&backupHour, "backup-hour-utc", 0, "час ежедневного бэкапа (0–23 UTC)")
	f.IntVar(&backupDays, "backup-retention-days", 7, "срок хранения автоматических копий (1–365 дней)")
	f.IntVar(&months, "period-months", 0, "оплаченный период в месяцах")
	f.IntVar(&days, "period-days", 0, "оплаченный период в днях")
	f.BoolVar(&autoRenew, "auto-renew", false, "продлевать автоматически")
	f.StringArrayVar(&ifaces, "interface", nil,
		"интерфейс: type=public | type=vpc,vpc_id=<id>[,floating_ip=true] (можно повторять)")
	for _, r := range []string{"name", "image", "plan", "cluster", "user"} {
		_ = cmd.MarkFlagRequired(r)
	}
	return cmd
}

func vmDeleteCommand(env *Env) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:         "delete <вм>",
		Short:       "Удалить ВМ",
		Annotations: ops("vms_delete_vm"),
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, project, id, err := env.vmTarget(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if err := confirm(cmd, yes, "Удалить ВМ %s вместе с диском?", args[0]); err != nil {
				return err
			}
			if err := callOK(c.VmsDeleteVmWithResponse(cmd.Context(), project, id)); err != nil {
				return err
			}
			return env.Printer.Message("ВМ %s удаляется.", args[0])
		},
	}
	addYes(cmd, &yes)
	return cmd
}

var powerColumns = []output.Column{
	output.Col("статус", "status"),
	output.Col("сообщение", "message"),
}

// vmStartCommand отделён от stop/restart, потому что запуск — единственное
// действие с телом запроса: у остановленной ВМ мог истечь оплаченный период,
// и включение продлевает его за деньги. Молча продлевать нельзя, поэтому
// --renew обязателен явно.
func vmStartCommand(env *Env) *cobra.Command {
	var renew bool
	cmd := &cobra.Command{
		Use:         "start <вм>",
		Short:       "Включить ВМ",
		Annotations: ops("vms_start_vm"),
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, project, id, err := env.vmTarget(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			v, err := call(c.VmsStartVmWithResponse(cmd.Context(), project, id, tatnet.VMStartRequest{
				Renew: optBool(cmd, "renew", renew),
			}))
			if err != nil {
				return err
			}
			return env.Printer.Object(v, powerColumns)
		},
	}
	cmd.Flags().BoolVar(&renew, "renew", false, "продлить оплаченный период, если он истёк (списание с баланса)")
	return cmd
}

func vmPowerCommand(env *Env, verb, short, op string) *cobra.Command {
	return &cobra.Command{
		Use:         verb + " <вм>",
		Short:       short,
		Annotations: ops(op),
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, project, id, err := env.vmTarget(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			var v any
			if verb == "stop" {
				v, err = call(c.VmsStopVmWithResponse(cmd.Context(), project, id))
			} else {
				v, err = call(c.VmsRestartVmWithResponse(cmd.Context(), project, id))
			}
			if err != nil {
				return err
			}
			return env.Printer.Object(v, powerColumns)
		},
	}
}

func vmBackupCommand(env *Env) *cobra.Command {
	cmd := &cobra.Command{Use: "backup", Aliases: []string{"backups"}, Short: "Бэкапы ВМ"}

	cols := []output.Column{
		output.Col("имя", "name"),
		output.Col("тип", "type"),
		output.Col("статус", "status"),
		output.Col("размер", "size_bytes"),
		output.WideCol("id", "id"),
		output.WideCol("ошибка", "error"),
	}

	var limit int
	list := &cobra.Command{
		Use:         "list <вм>",
		Short:       "Бэкапы ВМ",
		Annotations: ops("vms_list_backups"),
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, project, id, err := env.vmTarget(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			all, err := paginate(cmd.Context(), func(ctx context.Context, offset, size int) (any, error) {
				return call(c.VmsListBackupsWithResponse(ctx, project, id, &tatnet.VmsListBackupsParams{
					Limit: &size, Offset: &offset,
				}))
			}, limit, 0)
			if err != nil {
				return err
			}
			return env.Printer.List(all, cols)
		},
	}
	list.Flags().IntVar(&limit, "limit", 0, "не больше стольких записей (0 — все)")

	var name, description string
	create := &cobra.Command{
		Use:         "create <вм>",
		Short:       "Снять бэкап",
		Annotations: ops("vms_create_backup"),
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, project, id, err := env.vmTarget(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			v, err := call(c.VmsCreateBackupWithResponse(cmd.Context(), project, id, tatnet.V1BackupCreate{
				Name:        name,
				Description: optStr(cmd, "description", description),
			}))
			if err != nil {
				return err
			}
			return env.Printer.Object(v, cols)
		},
	}
	create.Flags().StringVar(&name, "name", "", "имя бэкапа (обязательно)")
	create.Flags().StringVar(&description, "description", "", "описание")
	_ = create.MarkFlagRequired("name")

	var yes bool
	del := &cobra.Command{
		Use:         "delete <вм> <бэкап-id>",
		Short:       "Удалить бэкап",
		Annotations: ops("vms_delete_backup"),
		Args:        cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, project, id, err := env.vmTarget(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if err := confirm(cmd, yes, "Удалить бэкап %s?", args[1]); err != nil {
				return err
			}
			if err := callOK(c.VmsDeleteBackupWithResponse(cmd.Context(), project, id, args[1])); err != nil {
				return err
			}
			return env.Printer.Message("Бэкап %s удалён.", args[1])
		},
	}
	addYes(del, &yes)

	cmd.AddCommand(list, create, del, vmBackupPolicyCommand(env))
	return cmd
}

// vmTarget — общая преамбула команд про одну ВМ.
func (e *Env) vmTarget(ctx context.Context, ref string) (*tatnet.ClientWithResponses, string, string, error) {
	c, err := e.Client()
	if err != nil {
		return nil, "", "", err
	}
	project, id, err := e.flatTarget(ctx, kindVM, ref,
		func(ctx context.Context, project string) ([]any, error) { return e.listVMs(ctx, project, 0) },
		func(ctx context.Context, id string) (any, error) { return call(c.VmsGetVmByIdWithResponse(ctx, id)) },
		"name", "hostname")
	if err != nil {
		return nil, "", "", err
	}
	return c, project, id, nil
}

// parseInterface разбирает --interface type=vpc,vpc_id=…,floating_ip=true.
func parseInterface(spec string) (tatnet.V1VMInterface, error) {
	iface := tatnet.V1VMInterface{}
	for _, kv := range strings.Split(spec, ",") {
		key, val, ok := strings.Cut(strings.TrimSpace(kv), "=")
		if !ok {
			return iface, fmt.Errorf("--interface %q: ожидается ключ=значение через запятую", spec)
		}
		switch key {
		case "type":
			iface.Type = val
		case "vpc_id":
			iface.VpcId = ptr(val)
		case "floating_ip", "dhcp4", "enable_ipv4", "enable_ipv6":
			b, err := strconv.ParseBool(val)
			if err != nil {
				return iface, fmt.Errorf("--interface %s=%q: ожидается true или false", key, val)
			}
			switch key {
			case "floating_ip":
				iface.FloatingIp = &b
			case "dhcp4":
				iface.Dhcp4 = &b
			case "enable_ipv4":
				iface.EnableIpv4 = &b
			case "enable_ipv6":
				iface.EnableIpv6 = &b
			}
		default:
			return iface, fmt.Errorf("--interface: неизвестный ключ %q", key)
		}
	}
	if iface.Type == "" {
		return iface, fmt.Errorf("--interface %q: не указан type", spec)
	}
	return iface, nil
}

// readFileOrStdin читает файл или стандартный ввод по «-».
func readFileOrStdin(path string) ([]byte, error) {
	if path == "-" {
		return io.ReadAll(os.Stdin)
	}
	return os.ReadFile(path)
}

// vmBackupPolicyCommand uses the same public API and authorization as the panel.
func vmBackupPolicyCommand(env *Env) *cobra.Command {
	var enabled bool
	var hour, days int
	cmd := &cobra.Command{Use: "policy <вм>", Short: "Посмотреть или настроить ежедневные автобэкапы", Annotations: ops("vms_get_backup_policy", "vms_set_backup_policy"), Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, project, id, err := env.vmTarget(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			path := "/projects/" + project + "/vms/" + id + "/backup-policy"
			method := "GET"
			var body []byte
			if cmd.Flags().Changed("enabled") || cmd.Flags().Changed("hour-utc") || cmd.Flags().Changed("retention-days") {
				// Fetch current state first so editing retention never disables the policy.
				status, _, raw, e := env.rawRequest(cmd, "GET", path, nil, nil)
				if e != nil {
					return e
				}
				if status >= 400 {
					return &APIError{Status: status, Detail: detailOf(raw), Body: raw}
				}
				var policy map[string]any
				if e = json.Unmarshal(raw, &policy); e != nil {
					return e
				}
				if cmd.Flags().Changed("enabled") {
					policy["enabled"] = enabled
				}
				if cmd.Flags().Changed("hour-utc") {
					policy["hour_utc"] = hour
				}
				if cmd.Flags().Changed("retention-days") {
					policy["retention_days"] = days
				}
				body, e = json.Marshal(policy)
				if e != nil {
					return e
				}
				method = "PUT"
			}
			status, _, raw, err := env.rawRequest(cmd, method, path, body, nil)
			if err != nil {
				return err
			}
			if status >= 400 {
				return &APIError{Status: status, Detail: detailOf(raw), Body: raw}
			}
			return printRaw(cmd, env, raw)
		}}
	cmd.Flags().BoolVar(&enabled, "enabled", false, "включить/выключить расписание")
	cmd.Flags().IntVar(&hour, "hour-utc", 0, "час ежедневного запуска, UTC (0–23)")
	cmd.Flags().IntVar(&days, "retention-days", 7, "срок хранения автоматических копий (1–365 дней)")
	return cmd
}

func vmInterfaceCommand(env *Env) *cobra.Command {
	parent := &cobra.Command{Use: "interface", Short: "Сетевые интерфейсы ВМ (изменения только на выключенной ВМ)"}
	var vpc string
	attach := &cobra.Command{Use: "attach <вм>", Short: "Подключить отдельный интерфейс к VPC своего региона",
		Annotations: ops("vms_attach_vpc_interface"), Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, project, id, err := env.vmTarget(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			body, err := json.Marshal(map[string]string{"vpc_id": vpc})
			if err != nil {
				return err
			}
			status, _, data, err := env.rawRequest(cmd, http.MethodPost, "/projects/"+url.PathEscape(project)+"/vms/"+url.PathEscape(id)+"/interfaces", body, nil)
			if err != nil {
				return err
			}
			result, err := decode(status, data, nil)
			if err != nil {
				return err
			}
			return env.Printer.Object(result, []output.Column{output.Col("статус", "status"), output.Col("интерфейс", "interface_id")})
		}}
	attach.Flags().StringVar(&vpc, "vpc", "", "UUID VPC того же аккаунта и региона")
	_ = attach.MarkFlagRequired("vpc")
	detach := &cobra.Command{Use: "detach <вм> <interface-id>", Short: "Отключить дополнительный VPC-интерфейс",
		Annotations: ops("vms_detach_vpc_interface"), Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, project, id, err := env.vmTarget(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			status, _, data, err := env.rawRequest(cmd, http.MethodDelete, "/projects/"+url.PathEscape(project)+"/vms/"+url.PathEscape(id)+"/interfaces/"+url.PathEscape(args[1]), nil, nil)
			if err != nil {
				return err
			}
			result, err := decode(status, data, nil)
			if err != nil {
				return err
			}
			return env.Printer.Object(result, []output.Column{output.Col("статус", "status")})
		}}
	parent.AddCommand(attach, detach)
	return parent
}
