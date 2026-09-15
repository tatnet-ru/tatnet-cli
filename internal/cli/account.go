package cli

import (
	"github.com/spf13/cobra"

	"github.com/tatnet-ru/tatnet-cli/internal/output"
)

func newAccountCommand(env *Env) *cobra.Command {
	return &cobra.Command{
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
