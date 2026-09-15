// Package cli — дерево команд tatnet поверх сгенерированного клиента /v1.
package cli

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/tatnet-ru/tatnet-go/tatnet"

	"github.com/tatnet-ru/tatnet-cli/internal/config"
	"github.com/tatnet-ru/tatnet-cli/internal/output"
)

// Аннотация команды: какие операции контракта /v1 она вызывает. По ней
// coverage_test сверяет дерево команд с контрактом — иначе выпавшая или
// переименованная операция обнаруживалась бы у клиента.
const opAnnotation = "tatnet.ops"

// ops помечает команду списком operationId через запятую.
func ops(names ...string) map[string]string {
	return map[string]string{opAnnotation: strings.Join(names, ",")}
}

type globalFlags struct {
	profile string
	output  string
	project string
	apiKey  string
	baseURL string
	timeout time.Duration
	debug   bool
}

// NewRootCommand собирает всё дерево команд.
func NewRootCommand(version string) *cobra.Command {
	var g globalFlags
	env := &Env{}

	root := &cobra.Command{
		Use:   "tatnet",
		Short: "Управление облаком TatNet из терминала",
		Long: "tatnet — клиент публичного API TatNet (/v1).\n\n" +
			"Ключ создаётся в панели (Аккаунт → API-ключи) и сохраняется\n" +
			"командой `tatnet auth login`. Ключ принадлежит одному аккаунту\n" +
			"и несёт свою политику прав, поэтому аккаунт нигде не указывается.\n\n" +
			"Операции, для которых ещё нет своей команды, доступны через\n" +
			"`tatnet api` — это прямой вызов любого адреса контракта.",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			return setup(cmd, env, &g)
		},
	}

	f := root.PersistentFlags()
	f.StringVar(&g.profile, "profile", "", "профиль из конфига (или $TATNET_PROFILE)")
	f.StringVarP(&g.output, "output", "o", "", "формат вывода: table, wide, json, yaml")
	f.StringVarP(&g.project, "project", "p", "", "проект: id или имя (или $TATNET_PROJECT)")
	f.StringVar(&g.apiKey, "api-key", "", "API-ключ на один вызов (или $TATNET_API_KEY)")
	f.StringVar(&g.baseURL, "base-url", "", "адрес API (или $TATNET_BASE_URL)")
	f.DurationVar(&g.timeout, "timeout", 60*time.Second, "таймаут запроса")
	f.BoolVar(&g.debug, "debug", false, "печатать запросы и ответы в stderr (ключ вырезается)")

	root.AddCommand(
		newAuthCommand(env),
		newProfileCommand(env),
		newAccountCommand(env),
		newProjectCommand(env),
		newVMCommand(env),
		newAppCommand(env),
		newDNSCommand(env),
		newS3Command(env),
		newPGCommand(env),
		newValkeyCommand(env),
		newSSHKeyCommand(env),
		newAPICommand(env),
	)
	return root
}

// setup разрешает реквизиты: флаг → переменная окружения → профиль.
func setup(cmd *cobra.Command, env *Env, g *globalFlags) error {
	cfg, warn, err := config.Load()
	if err != nil {
		return err
	}
	if warn != "" {
		fmt.Fprintln(os.Stderr, "предупреждение: "+warn)
	}

	name, profile := cfg.Get(first(g.profile, os.Getenv("TATNET_PROFILE")))

	format, err := output.ParseFormat(first(g.output, os.Getenv("TATNET_OUTPUT"), string(output.Table)))
	if err != nil {
		return err
	}

	env.Cfg = cfg
	env.ProfileName = name
	env.Profile = profile
	env.APIKey = first(g.apiKey, os.Getenv("TATNET_API_KEY"), profile.APIKey)
	env.BaseURL = first(g.baseURL, os.Getenv("TATNET_BASE_URL"), profile.BaseURL, tatnet.DefaultBaseURL)
	env.Project = first(g.project, os.Getenv("TATNET_PROJECT"), profile.Project)
	env.Timeout = g.timeout
	env.Debug = g.debug
	env.Printer = output.Printer{Out: cmd.OutOrStdout(), Format: format}
	env.client = nil
	return nil
}

// first возвращает первое непустое значение.
func first(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
