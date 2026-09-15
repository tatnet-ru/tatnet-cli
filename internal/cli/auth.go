package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/tatnet-ru/tatnet-cli/internal/config"
	"github.com/tatnet-ru/tatnet-cli/internal/output"
)

func newAuthCommand(env *Env) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Ключ доступа: вход, проверка, выход",
	}
	cmd.AddCommand(newAuthLoginCommand(env), newAuthStatusCommand(env), newAuthLogoutCommand(env))
	return cmd
}

func newAuthLoginCommand(env *Env) *cobra.Command {
	var keyFlag string
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Сохранить API-ключ в профиль",
		Long: "Спрашивает ключ, проверяет его вызовом /account и сохраняет в профиль.\n\n" +
			"Ключ проверяется ДО записи: сохранённый нерабочий ключ выглядел бы\n" +
			"как настроенный CLI и падал бы на первой же команде.",
		Annotations: ops("account_whoami"),
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			key := strings.TrimSpace(keyFlag)
			if key == "" {
				var err error
				if key, err = promptSecret(cmd, "API-ключ (ввод скрыт): "); err != nil {
					return err
				}
			}
			if key == "" {
				return fmt.Errorf("пустой ключ")
			}

			env.APIKey = key
			env.client = nil
			who, err := env.whoami(cmd.Context())
			if err != nil {
				return fmt.Errorf("ключ не принят, ничего не сохранено: %w", err)
			}

			p := env.Profile
			p.APIKey = key
			if env.BaseURL != "" {
				p.BaseURL = env.BaseURL
			}
			env.Cfg.Set(env.ProfileName, p)
			if env.Cfg.Current == "" {
				env.Cfg.Current = env.ProfileName
			}
			if err := env.Cfg.Save(); err != nil {
				return err
			}
			path, _ := config.Path()
			return env.Printer.Message("Ключ сохранён в профиль %q (%s).\nАккаунт: %s",
				env.ProfileName, path, output.Value(who, "account_id"))
		},
	}
	cmd.Flags().StringVar(&keyFlag, "key", "", "ключ значением, а не вводом (попадёт в историю оболочки)")
	return cmd
}

func newAuthStatusCommand(env *Env) *cobra.Command {
	return &cobra.Command{
		Use:         "status",
		Short:       "Чей ключ используется и что он может",
		Annotations: ops("account_whoami"),
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			who, err := env.whoami(cmd.Context())
			if err != nil {
				return err
			}
			if env.Printer.Format == output.JSON || env.Printer.Format == output.YAML {
				return env.Printer.Raw(who)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "профиль    %s\nадрес      %s\nключ из    %s\nаккаунт    %s\nид ключа   %s\n",
				env.ProfileName, env.BaseURL, env.keySource(), output.Value(who, "account_id"), output.Value(who, "key_id"))

			m, _ := who.(map[string]any)
			stmts, _ := m["policy"].([]any)
			if len(stmts) == 0 {
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), "\nполитика")
			return output.Printer{Out: cmd.OutOrStdout(), Format: output.Table}.List(stmts, []output.Column{
				output.Col("эффект", "effect"),
				output.Col("действия", "actions"),
				output.Col("ресурсы", "resources"),
			})
		},
	}
}

func newAuthLogoutCommand(env *Env) *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Убрать ключ из профиля",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			p := env.Profile
			if p.APIKey == "" {
				return env.Printer.Message("В профиле %q ключа и не было.", env.ProfileName)
			}
			p.APIKey = ""
			env.Cfg.Set(env.ProfileName, p)
			if err := env.Cfg.Save(); err != nil {
				return err
			}
			msg := fmt.Sprintf("Ключ убран из профиля %q.", env.ProfileName)
			if os.Getenv("TATNET_API_KEY") != "" {
				msg += "\nВнимание: в окружении задан TATNET_API_KEY — команды продолжат работать с ним."
			}
			return env.Printer.Message("%s", msg)
		},
	}
}

// whoami — единственный способ проверить ключ, не меняя ничего на стороне API.
func (e *Env) whoami(ctx context.Context) (any, error) {
	c, err := e.Client()
	if err != nil {
		return nil, err
	}
	return call(c.AccountWhoamiWithResponse(ctx))
}

// keySource говорит, откуда взят ключ: без этого «поменял переменную, а CLI
// ходит старым ключом» выясняется только отказом.
func (e *Env) keySource() string {
	switch {
	case e.APIKey == "":
		return "нигде не задан"
	case e.APIKey == os.Getenv("TATNET_API_KEY"):
		return "переменной TATNET_API_KEY"
	case e.APIKey == e.Profile.APIKey:
		return "профиля"
	default:
		return "флага --api-key"
	}
}

func promptSecret(cmd *cobra.Command, prompt string) (string, error) {
	fmt.Fprint(cmd.ErrOrStderr(), prompt)
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		b, err := term.ReadPassword(fd)
		fmt.Fprintln(cmd.ErrOrStderr())
		return strings.TrimSpace(string(b)), err
	}
	// Не терминал — читаем строку: так работает `echo "$KEY" | tatnet auth login`.
	line, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	return strings.TrimSpace(line), err
}
