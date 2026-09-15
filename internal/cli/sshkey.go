package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tatnet-ru/tatnet-go/tatnet"

	"github.com/tatnet-ru/tatnet-cli/internal/output"
)

func newSSHKeyCommand(env *Env) *cobra.Command {
	cmd := &cobra.Command{Use: "ssh-key", Aliases: []string{"ssh-keys"}, Short: "SSH-ключи аккаунта"}

	cols := []output.Column{
		output.Col("имя", "name"),
		output.Col("id", "id"),
		output.WideCol("ключ", "public_key"),
	}

	list := &cobra.Command{
		Use:         "list",
		Short:       "Список ключей",
		Annotations: ops("ssh_keys_list_ssh_keys"),
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			all, err := env.listSSHKeys(cmd.Context())
			if err != nil {
				return err
			}
			return env.Printer.List(all, cols)
		},
	}

	var name, keyFile, keyLiteral string
	create := &cobra.Command{
		Use:         "create",
		Short:       "Добавить ключ",
		Annotations: ops("ssh_keys_create_ssh_key"),
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, err := env.Client()
			if err != nil {
				return err
			}
			pub := strings.TrimSpace(keyLiteral)
			if pub == "" {
				if keyFile == "" {
					return fmt.Errorf("укажите --public-key-file или --public-key")
				}
				data, err := readFileOrStdin(keyFile)
				if err != nil {
					return fmt.Errorf("--public-key-file: %w", err)
				}
				pub = strings.TrimSpace(string(data))
			}
			if strings.Contains(pub, "PRIVATE KEY") {
				return fmt.Errorf("это приватный ключ — публичный лежит в файле .pub")
			}
			v, err := call(c.SshKeysCreateSshKeyWithResponse(cmd.Context(), tatnet.V1SshKeyCreate{
				Name: name, PublicKey: pub,
			}))
			if err != nil {
				return err
			}
			return env.Printer.Object(v, cols)
		},
	}
	create.Flags().StringVar(&name, "name", "", "имя ключа (обязательно)")
	create.Flags().StringVar(&keyFile, "public-key-file", "", "файл с публичным ключом (- для stdin)")
	create.Flags().StringVar(&keyLiteral, "public-key", "", "публичный ключ значением")
	_ = create.MarkFlagRequired("name")

	var yes bool
	del := &cobra.Command{
		Use:         "delete <ключ>",
		Short:       "Удалить ключ",
		Annotations: ops("ssh_keys_delete_ssh_key"),
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := env.Client()
			if err != nil {
				return err
			}
			id, err := resolveRef(cmd.Context(), "SSH-ключ", args[0], env.listSSHKeys, "name")
			if err != nil {
				return err
			}
			if err := confirm(cmd, yes, "Удалить SSH-ключ %s?", args[0]); err != nil {
				return err
			}
			if err := callOK(c.SshKeysDeleteSshKeyWithResponse(cmd.Context(), id)); err != nil {
				return err
			}
			return env.Printer.Message("Ключ %s удалён.", args[0])
		},
	}
	addYes(del, &yes)

	cmd.AddCommand(list, create, del)
	return cmd
}

func (e *Env) listSSHKeys(ctx context.Context) ([]any, error) {
	c, err := e.Client()
	if err != nil {
		return nil, err
	}
	return paginate(ctx, func(ctx context.Context, offset, size int) (any, error) {
		return call(c.SshKeysListSshKeysWithResponse(ctx, &tatnet.SshKeysListSshKeysParams{
			Limit: &size, Offset: &offset,
		}))
	}, 0, 0)
}
