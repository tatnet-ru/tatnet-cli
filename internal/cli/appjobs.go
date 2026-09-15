package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/tatnet-ru/tatnet-go/tatnet"

	"github.com/tatnet-ru/tatnet-cli/internal/output"
)

var jobColumns = []output.Column{
	output.Col("имя", "name"),
	output.Col("команда", "command"),
	output.Col("расписание", "schedule"),
	output.Col("включена", "enabled"),
	output.Col("таймаут", "timeout_seconds"),
	output.WideCol("id", "id"),
	output.WideCol("последний запуск", "last_run.status"),
}

var jobRunColumns = []output.Column{
	output.Col("статус", "status"),
	output.Col("вид", "kind"),
	output.Col("код", "exit_code"),
	output.Col("длительность", "duration_ms"),
	output.Col("начат", "started_at"),
	output.WideCol("id", "id"),
	output.WideCol("задача", "job_id"),
	output.WideCol("кем запущен", "triggered_by"),
	output.WideCol("ошибка", "error"),
}

func (e *Env) listJobs(ctx context.Context, c *tatnet.ClientWithResponses, project, app string) ([]any, error) {
	return paginate(ctx, func(ctx context.Context, offset, size int) (any, error) {
		return call(c.AppJobsV1ListJobsWithResponse(ctx, project, app, &tatnet.AppJobsV1ListJobsParams{
			Limit: &size, Offset: &offset,
		}))
	}, 0, 0)
}

func appJobCommand(env *Env) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "job",
		Aliases: []string{"jobs"},
		Short:   "Задачи backend-приложения",
		Long: "Задачи — команды, выполняемые в отдельной ВМ из артефакта приложения:\n" +
			"по расписанию (cron из пяти полей, время UTC) или вручную.\n" +
			"Только у приложений типа backend.",
	}

	list := &cobra.Command{
		Use:         "list <приложение>",
		Short:       "Задачи приложения",
		Annotations: ops("app_jobs_v1_list_jobs"),
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, project, id, err := env.appTarget(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			all, err := env.listJobs(cmd.Context(), c, project, id)
			if err != nil {
				return err
			}
			return env.Printer.List(all, jobColumns)
		},
	}

	var schedule string
	var timeout int
	var enabled bool
	create := &cobra.Command{
		Use:         "create <приложение> <имя> <команда>",
		Short:       "Создать задачу",
		Annotations: ops("app_jobs_v1_create_job"),
		Args:        cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, project, id, err := env.appTarget(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			v, err := call(c.AppJobsV1CreateJobWithResponse(cmd.Context(), project, id, tatnet.AppJobCreateRequest{
				Name:           args[1],
				Command:        args[2],
				Schedule:       optStr(cmd, "schedule", schedule),
				TimeoutSeconds: optInt(cmd, "timeout", timeout),
				Enabled:        optBool(cmd, "enabled", enabled),
			}))
			if err != nil {
				return err
			}
			return env.Printer.Object(v, jobColumns)
		},
	}
	create.Flags().StringVar(&schedule, "schedule", "", "cron из пяти полей, время UTC (без флага — только вручную)")
	create.Flags().IntVar(&timeout, "timeout", 0, "таймаут, с (10–3600)")
	create.Flags().BoolVar(&enabled, "enabled", true, "включена ли задача")

	var uname, ucommand, uschedule string
	var utimeout int
	var uenabled bool
	update := &cobra.Command{
		Use:         "update <приложение> <задача>",
		Short:       "Изменить задачу",
		Annotations: ops("app_jobs_v1_update_job"),
		Args:        cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, project, appID, jobID, err := env.jobTarget(cmd.Context(), args[0], args[1])
			if err != nil {
				return err
			}
			v, err := call(c.AppJobsV1UpdateJobWithResponse(cmd.Context(), project, appID, jobID,
				tatnet.AppJobUpdateRequest{
					Name:           optStr(cmd, "name", uname),
					Command:        optStr(cmd, "command", ucommand),
					Schedule:       optStr(cmd, "schedule", uschedule),
					TimeoutSeconds: optInt(cmd, "timeout", utimeout),
					Enabled:        optBool(cmd, "enabled", uenabled),
				}))
			if err != nil {
				return err
			}
			return env.Printer.Object(v, jobColumns)
		},
	}
	update.Flags().StringVar(&uname, "name", "", "новое имя")
	update.Flags().StringVar(&ucommand, "command", "", "новая команда")
	update.Flags().StringVar(&uschedule, "schedule", "", "новое расписание")
	update.Flags().IntVar(&utimeout, "timeout", 0, "таймаут, с")
	update.Flags().BoolVar(&uenabled, "enabled", true, "включена ли задача")

	var yes bool
	del := &cobra.Command{
		Use:         "delete <приложение> <задача>",
		Short:       "Удалить задачу",
		Annotations: ops("app_jobs_v1_delete_job"),
		Args:        cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, project, appID, jobID, err := env.jobTarget(cmd.Context(), args[0], args[1])
			if err != nil {
				return err
			}
			if err := confirm(cmd, yes, "Удалить задачу %s?", args[1]); err != nil {
				return err
			}
			if err := callOK(c.AppJobsV1DeleteJobWithResponse(cmd.Context(), project, appID, jobID)); err != nil {
				return err
			}
			return env.Printer.Message("Задача %s удалена.", args[1])
		},
	}
	addYes(del, &yes)

	run := &cobra.Command{
		Use:   "run <приложение> <задача>",
		Short: "Запустить задачу вручную",
		Long: "Ставит запуск в очередь региона и возвращает запись о нём.\n" +
			"Команда не ждёт завершения: ход смотрите в `tatnet app run list`.",
		Annotations: ops("app_jobs_v1_run_job"),
		Args:        cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, project, appID, jobID, err := env.jobTarget(cmd.Context(), args[0], args[1])
			if err != nil {
				return err
			}
			v, err := call(c.AppJobsV1RunJobWithResponse(cmd.Context(), project, appID, jobID))
			if err != nil {
				return err
			}
			return env.Printer.Object(v, jobRunColumns)
		},
	}

	cmd.AddCommand(list, create, update, del, run)
	return cmd
}

func appRunCommand(env *Env) *cobra.Command {
	cmd := &cobra.Command{Use: "run", Aliases: []string{"runs"}, Short: "Запуски задач"}

	var jobRef string
	var limit int
	list := &cobra.Command{
		Use:         "list <приложение>",
		Short:       "История запусков",
		Annotations: ops("app_jobs_v1_list_runs"),
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, project, id, err := env.appTarget(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			var jobID *string
			if jobRef != "" {
				resolved, err := resolveRef(cmd.Context(), "задача", jobRef, func(ctx context.Context) ([]any, error) {
					return env.listJobs(ctx, c, project, id)
				}, "name")
				if err != nil {
					return err
				}
				jobID = &resolved
			}
			all, err := paginate(cmd.Context(), func(ctx context.Context, offset, size int) (any, error) {
				return call(c.AppJobsV1ListRunsWithResponse(ctx, project, id, &tatnet.AppJobsV1ListRunsParams{
					JobId: jobID, Limit: &size, Offset: &offset,
				}))
			}, limit, 0)
			if err != nil {
				return err
			}
			return env.Printer.List(all, jobRunColumns)
		},
	}
	list.Flags().StringVar(&jobRef, "job", "", "только запуски этой задачи")
	list.Flags().IntVar(&limit, "limit", 20, "не больше стольких запусков (0 — все)")

	var logs bool
	get := &cobra.Command{
		Use:         "get <приложение> <запуск-id>",
		Short:       "Один запуск",
		Annotations: ops("app_jobs_v1_get_run"),
		Args:        cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, project, id, err := env.appTarget(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			v, err := call(c.AppJobsV1GetRunWithResponse(cmd.Context(), project, id, args[1]))
			if err != nil {
				return err
			}
			if logs && env.Printer.Format != output.JSON && env.Printer.Format != output.YAML {
				tail := output.Value(v, "log_tail")
				if tail == "-" {
					return fmt.Errorf("у запуска %s нет сохранённого вывода", args[1])
				}
				fmt.Fprintln(cmd.OutOrStdout(), tail)
				return nil
			}
			return env.Printer.Object(v, append(jobRunColumns,
				output.Col("команда", "command"),
				output.Col("хвост вывода", "log_tail"),
			))
		},
	}
	get.Flags().BoolVar(&logs, "logs", false, "печатать только сохранённый вывод")

	cmd.AddCommand(list, get)
	return cmd
}

// jobTarget разрешает приложение и задачу разом.
func (e *Env) jobTarget(ctx context.Context, appRef, jobRef string) (*tatnet.ClientWithResponses, string, string, string, error) {
	c, project, appID, err := e.appTarget(ctx, appRef)
	if err != nil {
		return nil, "", "", "", err
	}
	jobID, err := resolveRef(ctx, "задача", jobRef, func(ctx context.Context) ([]any, error) {
		return e.listJobs(ctx, c, project, appID)
	}, "name")
	if err != nil {
		return nil, "", "", "", err
	}
	return c, project, appID, jobID, nil
}
