package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"github.com/tatnet-ru/tatnet-cli/internal/config"
	"github.com/tatnet-ru/tatnet-cli/internal/output"
)

func newProfileCommand(env *Env) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profile",
		Short: "Профили: несколько аккаунтов или адресов в одном конфиге",
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "list",
			Short: "Показать профили",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				names := env.Cfg.Names()
				sort.Strings(names)
				rows := make([]any, 0, len(names))
				for _, n := range names {
					p := env.Cfg.Profiles[n]
					current := ""
					if n == env.ProfileName {
						current = "←"
					}
					rows = append(rows, map[string]any{
						"name": n, "current": current, "base_url": p.BaseURL,
						"project": p.Project, "key": maskKey(p.APIKey),
					})
				}
				return env.Printer.ListLocal(rows, []output.Column{
					output.Col("", "current"),
					output.Col("профиль", "name"),
					output.Col("ключ", "key"),
					output.Col("проект", "project"),
					output.Col("адрес", "base_url"),
				})
			},
		},
		&cobra.Command{
			Use:   "use <профиль>",
			Short: "Сделать профиль текущим",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				if _, ok := env.Cfg.Profiles[args[0]]; !ok {
					env.Cfg.Set(args[0], config.Profile{})
				}
				env.Cfg.Current = args[0]
				if err := env.Cfg.Save(); err != nil {
					return err
				}
				return env.Printer.Message("Текущий профиль: %s", args[0])
			},
		},
		&cobra.Command{
			Use:   "set-project <проект>",
			Short: "Проект по умолчанию для текущего профиля",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				id, err := env.ResolveProject(cmd.Context(), args[0])
				if err != nil {
					return err
				}
				p := env.Profile
				p.Project = id
				env.Cfg.Set(env.ProfileName, p)
				if err := env.Cfg.Save(); err != nil {
					return err
				}
				return env.Printer.Message("Проект по умолчанию для профиля %q: %s", env.ProfileName, id)
			},
		},
		&cobra.Command{
			Use:   "path",
			Short: "Где лежит конфиг",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				p, err := config.Path()
				if err != nil {
					return err
				}
				return env.Printer.Raw(p)
			},
		},
	)
	return cmd
}

// maskKey показывает ровно столько, чтобы отличить один ключ от другого.
func maskKey(k string) string {
	if k == "" {
		return "нет"
	}
	if len(k) <= 12 {
		return "задан"
	}
	return fmt.Sprintf("%s…%s", k[:11], k[len(k)-4:])
}
