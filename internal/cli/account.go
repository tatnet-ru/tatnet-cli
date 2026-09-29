package cli

import (
	"github.com/spf13/cobra"

	"github.com/tatnet-ru/tatnet-cli/internal/output"
)

func newAccountCommand(env *Env) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "account",
		Short:       "Аккаунт, которому принадлежит ключ",
		Annotations: ops("account_whoami"),
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			who, err := env.whoami(cmd.Context())
			if err != nil {
				return err
			}
			return env.Printer.Object(who, []output.Column{
				output.Col("аккаунт", "account_id"),
				output.Col("ключ", "key_id"),
			})
		},
	}
	cmd.AddCommand(newAccountBalanceCommand(env))
	return cmd
}

func newAccountBalanceCommand(env *Env) *cobra.Command {
	return &cobra.Command{
		Use:   "balance",
		Short: "Баланс аккаунта",
		Long: `Баланс аккаунта, которому принадлежит ключ.

«доступно» — на что можно купить платные ресурсы прямо сейчас: деньги
плюс бонусы, если они уже открыты. Бонусы идут в оплату только после
первого реального пополнения — пока его не было, команда покажет, сколько
пополнить.

Нужно действие billing:read: оно есть только у ключа владельца аккаунта.
Оплатить или пополнить счёт ключом нельзя — это делается в панели.`,
		Annotations: ops("account_get_balance"),
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := env.Client()
			if err != nil {
				return err
			}
			v, err := call(c.AccountGetBalanceWithResponse(cmd.Context()))
			if err != nil {
				return err
			}
			cols := []output.Column{
				output.Col("доступно", "available"),
				output.Col("баланс", "balance"),
				output.Col("бонусы", "credits"),
				output.Col("бонусы в оплату", "credits_unlocked"),
				output.Col("валюта", "currency"),
			}
			if !output.Bool(v, "credits_unlocked") {
				cols = append(cols, output.Col("пополнить, чтобы открыть бонусы", "real_topup_min"))
			}
			return env.Printer.Object(v, cols)
		},
	}
}

func newProjectCommand(env *Env) *cobra.Command {
	cmd := &cobra.Command{Use: "project", Aliases: []string{"projects"}, Short: "Проекты аккаунта"}

	cols := []output.Column{
		output.Col("имя", "name"),
		output.Col("id", "id"),
		output.Col("описание", "description"),
	}

	list := &cobra.Command{
		Use:         "list",
		Short:       "Список проектов",
		Annotations: ops("projects_list_projects"),
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			all, err := env.listProjects(cmd.Context())
			if err != nil {
				return err
			}
			return env.Printer.List(all, cols)
		},
	}

	get := &cobra.Command{
		Use:         "get <проект>",
		Short:       "Один проект",
		Annotations: ops("projects_get_project"),
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := env.Client()
			if err != nil {
				return err
			}
			id, err := env.ResolveProject(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			v, err := call(c.ProjectsGetProjectWithResponse(cmd.Context(), id))
			if err != nil {
				return err
			}
			return env.Printer.Object(v, append(cols, output.Col("эмодзи", "emoji")))
		},
	}

	cmd.AddCommand(list, get)
	return cmd
}
