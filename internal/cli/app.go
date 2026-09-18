package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tatnet-ru/tatnet-go/tatnet"

	"github.com/tatnet-ru/tatnet-cli/internal/output"
)

var appColumns = []output.Column{
	output.Col("имя", "name"),
	output.Col("тип", "app_type"),
	output.Col("статус", "status"),
	output.Col("деплой", "deploy_state"),
	output.Col("реплик", "replica_count"),
	output.Col("репозиторий", "repo_full_name"),
	output.Col("ветка", "branch"),
	output.WideCol("проект", "project_id"),
	output.WideCol("id", "id"),
	output.WideCol("автодеплой", "auto_deploy"),
	output.WideCol("образ", "docker_image"),
	output.WideCol("ошибка источника", "source_error"),
}

func newAppCommand(env *Env) *cobra.Command {
	cmd := &cobra.Command{Use: "app", Aliases: []string{"apps"}, Short: "Приложения Apps Platform"}
	cmd.AddCommand(
		appListCommand(env),
		appGetCommand(env),
		appCreateCommand(env),
		appUpdateCommand(env),
		appDeleteCommand(env),
		appDeployCommand(env),
		appBuildCommand(env),
		appEnvCommand(env),
		appDomainCommand(env),
		appJobCommand(env),
		appRunCommand(env),
	)
	return cmd
}

// listApps — приложения аккаунта ключа через плоский GET /apps. Пустой
// project — весь аккаунт; иначе фильтр ?project_id=.
//
// Раньше список шёл через /projects/{id}/apps, и без проекта команды про
// приложение были бессильны: ключу, которому не дали GET /projects, был
// недоступен сам адрес ресурса, а не ресурс (api#1060). Плоский путь снимает
// это: приложение находится по имени, проект узнаётся из него.
func (e *Env) listApps(ctx context.Context, project string, limit int) ([]any, error) {
	c, err := e.Client()
	if err != nil {
		return nil, err
	}
	var pid *string
	if project != "" {
		pid = &project
	}
	return paginate(ctx, func(ctx context.Context, offset, size int) (any, error) {
		return call(c.AppsListAppsByAccountWithResponse(ctx, &tatnet.AppsListAppsByAccountParams{
			ProjectId: pid, Limit: &size, Offset: &offset,
		}))
	}, limit, 0)
}

// optionalProject — проект, если задан (флагом, переменной, профилем), иначе
// пустая строка. Для команд про одно приложение проект стал необязательным:
// он сужает поиск по имени, но не нужен, чтобы приложение найти.
func (e *Env) optionalProject(ctx context.Context) (string, error) {
	if strings.TrimSpace(e.Project) == "" {
		return "", nil
	}
	return e.ResolveProject(ctx, e.Project)
}

// appTarget — общая преамбула команд про одно приложение: клиент, проект и
// id. Проект берётся из самого приложения, а не требуется заранее.
func (e *Env) appTarget(ctx context.Context, ref string) (*tatnet.ClientWithResponses, string, string, error) {
	c, err := e.Client()
	if err != nil {
		return nil, "", "", err
	}
	project, err := e.optionalProject(ctx)
	if err != nil {
		return nil, "", "", err
	}
	ref = strings.TrimSpace(ref)
	if IsID(ref) {
		// По id приложение достаётся напрямую, без списка и без проекта.
		app, err := call(c.AppsGetAppByIdWithResponse(ctx, ref))
		if err != nil {
			return nil, "", "", err
		}
		return c, output.Value(app, "project_id"), ref, nil
	}
	all, err := e.listApps(ctx, project, 0)
	if err != nil {
		return nil, "", "", fmt.Errorf("не удалось найти приложение %q: %w", ref, err)
	}
	id, err := resolveRef(ctx, kindApp, ref, func(context.Context) ([]any, error) { return all, nil }, "name")
	if err != nil {
		return nil, "", "", err
	}
	for _, it := range all {
		if output.Value(it, "id") == id {
			return c, output.Value(it, "project_id"), id, nil
		}
	}
	return nil, "", "", fmt.Errorf("приложение %q найдено, но без проекта в ответе", ref)
}

func appListCommand(env *Env) *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "Приложения аккаунта (или проекта, если он задан)",
		Annotations: ops("apps_list_apps_by_account"),
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			project, err := env.optionalProject(cmd.Context())
			if err != nil {
				return err
			}
			all, err := env.listApps(cmd.Context(), project, limit)
			if err != nil {
				return err
			}
			return env.Printer.List(all, appColumns)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 0, "не больше стольких записей (0 — все)")
	return cmd
}

func appGetCommand(env *Env) *cobra.Command {
	return &cobra.Command{
		Use:         "get <приложение>",
		Short:       "Одно приложение",
		Annotations: ops("apps_get_app"),
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, project, id, err := env.appTarget(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			v, err := call(c.AppsGetAppWithResponse(cmd.Context(), project, id))
			if err != nil {
				return err
			}
			return env.Printer.Object(v, append(appColumns,
				output.Col("текущая сборка", "current_build_id"),
				output.Col("pre-deploy", "predeploy_command"),
				output.Col("путь готовности", "readiness_path"),
			))
		},
	}
}

func appCreateCommand(env *Env) *cobra.Command {
	var (
		name, appType, runtime, framework      string
		repo, branch, provider, dockerImage    string
		buildCmd, installCmd, startCmd, outDir string
		rootDir, predeploy, readiness          string
		connection, cluster, vpc, nodejs       string
		sourceType                             string
		replicas, vcpu, memory                 int
		autoDeploy                             bool
	)
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Создать приложение",
		Annotations: ops("apps_create_app"),
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
			v, err := call(c.AppsCreateAppWithResponse(cmd.Context(), project, tatnet.V1AppCreate{
				Name:             name,
				AppType:          optStr(cmd, "type", appType),
				RuntimeType:      optStr(cmd, "runtime", runtime),
				Framework:        optStr(cmd, "framework", framework),
				SourceType:       optStr(cmd, "source-type", sourceType),
				RepoFullName:     optStr(cmd, "repo", repo),
				Branch:           optStr(cmd, "branch", branch),
				GitProvider:      optStr(cmd, "git-provider", provider),
				GitConnectionId:  optStr(cmd, "git-connection", connection),
				DockerImage:      optStr(cmd, "docker-image", dockerImage),
				BuildCommand:     optStr(cmd, "build-command", buildCmd),
				InstallCommand:   optStr(cmd, "install-command", installCmd),
				StartCommand:     optStr(cmd, "start-command", startCmd),
				OutputDirectory:  optStr(cmd, "output-directory", outDir),
				RootDirectory:    optStr(cmd, "root-directory", rootDir),
				PredeployCommand: optStr(cmd, "predeploy-command", predeploy),
				ReadinessPath:    optStr(cmd, "readiness-path", readiness),
				NodejsVersion:    optStr(cmd, "nodejs-version", nodejs),
				ClusterId:        optStr(cmd, "cluster", cluster),
				VpcId:            optStr(cmd, "vpc", vpc),
				ReplicaCount:     optInt(cmd, "replicas", replicas),
				ReplicaVcpu:      optInt(cmd, "vcpu", vcpu),
				ReplicaMemoryMb:  optInt(cmd, "memory", memory),
				AutoDeploy:       optBool(cmd, "auto-deploy", autoDeploy),
			}))
			if err != nil {
				return err
			}
			return env.Printer.Object(v, appColumns)
		},
	}
	f := cmd.Flags()
	f.StringVar(&name, "name", "", "имя приложения (обязательно)")
	f.StringVar(&appType, "type", "", "тип: static, ssr, backend")
	f.StringVar(&runtime, "runtime", "", "рантайм")
	f.StringVar(&framework, "framework", "", "фреймворк")
	f.StringVar(&sourceType, "source-type", "", "источник: git или docker_image")
	f.StringVar(&repo, "repo", "", "репозиторий вида owner/name")
	f.StringVar(&branch, "branch", "", "ветка")
	f.StringVar(&provider, "git-provider", "", "провайдер: github, gitlab, gitea, gitverse, gitflic")
	f.StringVar(&connection, "git-connection", "", "id подключения к провайдеру")
	f.StringVar(&dockerImage, "docker-image", "", "образ Docker (для backend из образа)")
	f.StringVar(&buildCmd, "build-command", "", "команда сборки")
	f.StringVar(&installCmd, "install-command", "", "команда установки зависимостей")
	f.StringVar(&startCmd, "start-command", "", "команда запуска")
	f.StringVar(&outDir, "output-directory", "", "каталог сборки")
	f.StringVar(&rootDir, "root-directory", "", "корень приложения в репозитории")
	f.StringVar(&predeploy, "predeploy-command", "", "команда до выката (миграции)")
	f.StringVar(&readiness, "readiness-path", "", "путь проверки готовности")
	f.StringVar(&nodejs, "nodejs-version", "", "версия Node.js")
	f.StringVar(&cluster, "cluster", "", "id региона")
	f.StringVar(&vpc, "vpc", "", "id приватной сети")
	f.IntVar(&replicas, "replicas", 0, "число реплик")
	f.IntVar(&vcpu, "vcpu", 0, "vCPU на реплику")
	f.IntVar(&memory, "memory", 0, "память на реплику, МБ")
	f.BoolVar(&autoDeploy, "auto-deploy", false, "катить при пуше в ветку")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

func appUpdateCommand(env *Env) *cobra.Command {
	var (
		name, predeploy, readiness, vpc string
		replicas, vcpu, memory          int
	)
	cmd := &cobra.Command{
		Use:         "update <приложение>",
		Short:       "Изменить приложение",
		Annotations: ops("apps_update_app_route"),
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, project, id, err := env.appTarget(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			v, err := call(c.AppsUpdateAppRouteWithResponse(cmd.Context(), project, id, tatnet.V1AppUpdate{
				Name:             optStr(cmd, "name", name),
				PredeployCommand: optStr(cmd, "predeploy-command", predeploy),
				ReadinessPath:    optStr(cmd, "readiness-path", readiness),
				VpcId:            optStr(cmd, "vpc", vpc),
				ReplicaCount:     optInt(cmd, "replicas", replicas),
				ReplicaVcpu:      optInt(cmd, "vcpu", vcpu),
				ReplicaMemoryMb:  optInt(cmd, "memory", memory),
			}))
			if err != nil {
				return err
			}
			return env.Printer.Object(v, appColumns)
		},
	}
	f := cmd.Flags()
	f.StringVar(&name, "name", "", "новое имя")
	f.StringVar(&predeploy, "predeploy-command", "", "команда до выката")
	f.StringVar(&readiness, "readiness-path", "", "путь проверки готовности")
	f.StringVar(&vpc, "vpc", "", "id приватной сети")
	f.IntVar(&replicas, "replicas", 0, "число реплик")
	f.IntVar(&vcpu, "vcpu", 0, "vCPU на реплику")
	f.IntVar(&memory, "memory", 0, "память на реплику, МБ")
	return cmd
}

func appDeleteCommand(env *Env) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:         "delete <приложение>",
		Short:       "Удалить приложение",
		Annotations: ops("apps_delete_app_route"),
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, project, id, err := env.appTarget(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if err := confirm(cmd, yes, "Удалить приложение %s?", args[0]); err != nil {
				return err
			}
			if err := callOK(c.AppsDeleteAppRouteWithResponse(cmd.Context(), project, id)); err != nil {
				return err
			}
			return env.Printer.Message("Приложение %s удалено.", args[0])
		},
	}
	addYes(cmd, &yes)
	return cmd
}

func appDeployCommand(env *Env) *cobra.Command {
	var commit string
	cmd := &cobra.Command{
		Use:         "deploy <приложение>",
		Short:       "Запустить сборку и выкат",
		Annotations: ops("apps_deploy_app"),
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, project, id, err := env.appTarget(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			v, err := call(c.AppsDeployAppWithResponse(cmd.Context(), project, id, tatnet.V1DeployRequest{
				CommitSha: optStr(cmd, "commit", commit),
			}))
			if err != nil {
				return err
			}
			return env.Printer.Object(v, []output.Column{
				output.Col("сборка", "id"),
				output.Col("статус", "status"),
				output.Col("приложение", "app_id"),
				output.WideCol("сайт", "site_id"),
			})
		},
	}
	cmd.Flags().StringVar(&commit, "commit", "", "конкретный коммит")
	return cmd
}

func appBuildCommand(env *Env) *cobra.Command {
	cmd := &cobra.Command{Use: "build", Aliases: []string{"builds"}, Short: "Сборки приложения"}
	var limit int
	list := &cobra.Command{
		Use:         "list <приложение>",
		Short:       "История сборок",
		Annotations: ops("apps_list_builds"),
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, project, id, err := env.appTarget(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			all, err := paginate(cmd.Context(), func(ctx context.Context, offset, size int) (any, error) {
				return call(c.AppsListBuildsWithResponse(ctx, project, id, &tatnet.AppsListBuildsParams{
					Limit: &size, Offset: &offset,
				}))
			}, limit, 0)
			if err != nil {
				return err
			}
			return env.Printer.List(all, []output.Column{
				output.Col("статус", "status"),
				output.Col("коммит", "commit_sha"),
				// У сборки из папки коммита нет — вместо него отпечаток
				// архива. Без него «что сейчас в проде» осталось бы без
				// ответа: две выкладки одной папки неотличимы, а разные —
				// не видно, что разные.
				output.ShortCol("отпечаток", "source_archive_sha256", 12),
				output.Col("сообщение", "commit_message"),
				output.Col("длительность", "duration_ms"),
				output.Col("создана", "created_at"),
				output.WideCol("id", "id"),
				output.WideCol("ошибка", "error"),
				output.WideCol("ошибка запуска", "boot_error"),
			})
		},
	}
	list.Flags().IntVar(&limit, "limit", 20, "не больше стольких сборок (0 — все)")
	cmd.AddCommand(list)
	return cmd
}

func appEnvCommand(env *Env) *cobra.Command {
	cmd := &cobra.Command{Use: "env", Short: "Переменные окружения приложения"}

	envColumns := []output.Column{
		output.Col("имя", "name"),
		output.Col("значение", "value"),
		output.Col("секрет", "is_secret"),
		output.WideCol("id", "id"),
	}

	listVars := func(ctx context.Context, c *tatnet.ClientWithResponses, project, app string) ([]any, error) {
		return paginate(ctx, func(ctx context.Context, offset, size int) (any, error) {
			return call(c.AppsListEnvVarsWithResponse(ctx, project, app, &tatnet.AppsListEnvVarsParams{
				Limit: &size, Offset: &offset,
			}))
		}, 0, 0)
	}

	list := &cobra.Command{
		Use:         "list <приложение>",
		Short:       "Переменные приложения",
		Annotations: ops("apps_list_env_vars"),
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, project, id, err := env.appTarget(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			all, err := listVars(cmd.Context(), c, project, id)
			if err != nil {
				return err
			}
			return env.Printer.List(all, envColumns)
		},
	}

	var secret bool
	set := &cobra.Command{
		Use:   "set <приложение> <имя> <значение>",
		Short: "Добавить переменную",
		Long: "Добавляет переменную окружения.\n\n" +
			"⚠ Переменные вида NEXT_PUBLIC_* вшиваются в код на СБОРКЕ: чтобы\n" +
			"новое значение попало в приложение, нужна новая сборка\n" +
			"(`tatnet app deploy`), а не только перезапуск реплик.",
		Annotations: ops("apps_add_env_var"),
		Args:        cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, project, id, err := env.appTarget(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			v, err := call(c.AppsAddEnvVarWithResponse(cmd.Context(), project, id, tatnet.V1EnvVarCreate{
				Name: args[1], Value: args[2], IsSecret: optBool(cmd, "secret", secret),
			}))
			if err != nil {
				return err
			}
			return env.Printer.Object(v, envColumns)
		},
	}
	set.Flags().BoolVar(&secret, "secret", false, "хранить как секрет")

	var yes bool
	unset := &cobra.Command{
		Use:         "unset <приложение> <имя|id>",
		Short:       "Удалить переменную",
		Annotations: ops("apps_delete_env_var_route"),
		Args:        cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, project, id, err := env.appTarget(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			varID, err := resolveRef(cmd.Context(), kindEnvVar, args[1], func(ctx context.Context) ([]any, error) {
				return listVars(ctx, c, project, id)
			}, "name")
			if err != nil {
				return err
			}
			if err := confirm(cmd, yes, "Удалить переменную %s у %s?", args[1], args[0]); err != nil {
				return err
			}
			if err := callOK(c.AppsDeleteEnvVarRouteWithResponse(cmd.Context(), project, id, varID)); err != nil {
				return err
			}
			return env.Printer.Message("Переменная %s удалена.", args[1])
		},
	}
	addYes(unset, &yes)

	cmd.AddCommand(list, set, unset)
	return cmd
}

func appDomainCommand(env *Env) *cobra.Command {
	cmd := &cobra.Command{Use: "domain", Aliases: []string{"domains"}, Short: "Домены приложения"}

	domainColumns := []output.Column{
		output.Col("домен", "domain"),
		output.Col("тип", "type"),
		output.Col("статус", "status"),
		output.Col("dns настроен", "dns_configured"),
		output.Col("по умолчанию", "is_default"),
		output.WideCol("id", "id"),
		output.WideCol("адрес эджа", "target_ip"),
		output.WideCol("ошибка", "error"),
	}

	listDomains := func(ctx context.Context, c *tatnet.ClientWithResponses, project, app string) ([]any, error) {
		return paginate(ctx, func(ctx context.Context, offset, size int) (any, error) {
			return call(c.AppsListDomainsWithResponse(ctx, project, app, &tatnet.AppsListDomainsParams{
				Limit: &size, Offset: &offset,
			}))
		}, 0, 0)
	}

	list := &cobra.Command{
		Use:         "list <приложение>",
		Short:       "Домены приложения",
		Annotations: ops("apps_list_domains"),
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, project, id, err := env.appTarget(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			all, err := listDomains(cmd.Context(), c, project, id)
			if err != nil {
				return err
			}
			return env.Printer.List(all, domainColumns)
		},
	}

	var overrideDNS, skipCert bool
	add := &cobra.Command{
		Use:         "add <приложение> <домен>",
		Short:       "Привязать домен",
		Annotations: ops("apps_add_domain"),
		Args:        cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, project, id, err := env.appTarget(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			v, err := call(c.AppsAddDomainWithResponse(cmd.Context(), project, id, tatnet.V1DomainCreate{
				Domain:          args[1],
				OverrideDns:     optBool(cmd, "override-dns", overrideDNS),
				SkipCertificate: optBool(cmd, "skip-certificate", skipCert),
			}))
			if err != nil {
				return err
			}
			return env.Printer.Object(v, domainColumns)
		},
	}
	add.Flags().BoolVar(&overrideDNS, "override-dns", false, "перезаписать существующую A-запись в управляемой зоне")
	add.Flags().BoolVar(&skipCert, "skip-certificate", false, "не выпускать сертификат")

	var yes bool
	remove := &cobra.Command{
		Use:         "remove <приложение> <домен>",
		Short:       "Отвязать домен",
		Annotations: ops("apps_remove_domain"),
		Args:        cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, project, id, err := env.appTarget(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			domainID, err := resolveRef(cmd.Context(), kindDomain, args[1], func(ctx context.Context) ([]any, error) {
				return listDomains(ctx, c, project, id)
			}, "domain")
			if err != nil {
				return err
			}
			if err := confirm(cmd, yes, "Отвязать домен %s от %s?", args[1], args[0]); err != nil {
				return err
			}
			if err := callOK(c.AppsRemoveDomainWithResponse(cmd.Context(), project, id, domainID)); err != nil {
				return err
			}
			return env.Printer.Message("Домен %s отвязан.", args[1])
		},
	}
	addYes(remove, &yes)

	cmd.AddCommand(list, add, remove)
	return cmd
}
