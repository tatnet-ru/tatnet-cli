package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/tatnet-ru/tatnet-go/tatnet"

	"github.com/tatnet-ru/tatnet-cli/internal/output"
)

var bucketColumns = []output.Column{
	output.Col("имя", "name"),
	output.Col("регион", "region"),
	output.Col("статус", "status"),
	output.Col("публичный", "is_public"),
	output.Col("версии", "versioning"),
	output.Col("квота", "quota_bytes"),
	output.WideCol("id", "id"),
	output.WideCol("публичный адрес", "public_base_url"),
}

var accessKeyColumns = []output.Column{
	output.Col("имя", "name"),
	output.Col("ключ", "access_key_id"),
	output.Col("статус", "status"),
	output.Col("все бакеты", "all_buckets"),
	output.Col("бакеты", "buckets"),
	output.WideCol("id", "id"),
	output.WideCol("использован", "last_used_at"),
}

func newS3Command(env *Env) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "s3",
		Aliases: []string{"object-storage"},
		Short:   "Объектное хранилище S3",
	}
	cmd.AddCommand(
		s3BucketCommand(env),
		s3ObjectCommand(env),
		s3KeyCommand(env),
		&cobra.Command{
			Use:         "usage",
			Short:       "Занятое место по бакетам",
			Annotations: ops("object_storage_get_usage"),
			Args:        cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				c, err := env.Client()
				if err != nil {
					return err
				}
				v, err := call(c.ObjectStorageGetUsageWithResponse(cmd.Context()))
				if err != nil {
					return err
				}
				if env.Printer.Format == output.JSON || env.Printer.Format == output.YAML {
					return env.Printer.Raw(v)
				}
				m, _ := v.(map[string]any)
				buckets, _ := m["buckets"].([]any)
				fmt.Fprintf(cmd.OutOrStdout(), "всего байт    %s\nвсего объектов %s\n\n",
					output.Value(v, "total_bytes_stored"), output.Value(v, "total_object_count"))
				return env.Printer.List(buckets, []output.Column{
					output.Col("бакет", "name"),
					output.Col("байт", "bytes_stored"),
					output.Col("объектов", "object_count"),
					output.Col("квота", "quota_bytes"),
					output.WideCol("снимок", "snapshot_at"),
				})
			},
		},
		&cobra.Command{
			Use:         "connection-info",
			Aliases:     []string{"endpoint"},
			Short:       "Адрес и примеры подключения",
			Annotations: ops("object_storage_get_connection_info"),
			Args:        cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				c, err := env.Client()
				if err != nil {
					return err
				}
				v, err := call(c.ObjectStorageGetConnectionInfoWithResponse(cmd.Context()))
				if err != nil {
					return err
				}
				return env.Printer.Object(v, []output.Column{
					output.Col("адрес", "endpoint"),
					output.Col("регион", "region"),
					output.Col("path-style", "path_style_example"),
					output.Col("virtual-hosted", "virtual_hosted_example"),
				})
			},
		},
	)
	return cmd
}

func (e *Env) listBuckets(ctx context.Context) ([]any, error) {
	c, err := e.Client()
	if err != nil {
		return nil, err
	}
	return paginate(ctx, func(ctx context.Context, offset, size int) (any, error) {
		return call(c.ObjectStorageListBucketsWithResponse(ctx, &tatnet.ObjectStorageListBucketsParams{
			Limit: &size, Offset: &offset,
		}))
	}, 0, 0)
}

func (e *Env) bucketTarget(ctx context.Context, ref string) (*tatnet.ClientWithResponses, string, error) {
	c, err := e.Client()
	if err != nil {
		return nil, "", err
	}
	id, err := resolveRef(ctx, "бакет", ref, e.listBuckets, "name")
	if err != nil {
		return nil, "", err
	}
	return c, id, nil
}

func s3BucketCommand(env *Env) *cobra.Command {
	cmd := &cobra.Command{Use: "bucket", Aliases: []string{"buckets"}, Short: "Бакеты"}

	list := &cobra.Command{
		Use:         "list",
		Short:       "Список бакетов",
		Annotations: ops("object_storage_list_buckets"),
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			all, err := env.listBuckets(cmd.Context())
			if err != nil {
				return err
			}
			return env.Printer.List(all, bucketColumns)
		},
	}

	get := &cobra.Command{
		Use:         "get <бакет>",
		Short:       "Один бакет",
		Annotations: ops("object_storage_get_bucket"),
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, id, err := env.bucketTarget(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			v, err := call(c.ObjectStorageGetBucketWithResponse(cmd.Context(), id))
			if err != nil {
				return err
			}
			return env.Printer.Object(v, bucketColumns)
		},
	}

	var public, versioning bool
	var quota int
	create := &cobra.Command{
		Use:         "create <имя>",
		Short:       "Создать бакет",
		Annotations: ops("object_storage_create_bucket"),
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := env.Client()
			if err != nil {
				return err
			}
			v, err := call(c.ObjectStorageCreateBucketWithResponse(cmd.Context(), tatnet.V1BucketCreate{
				Name:       args[0],
				IsPublic:   optBool(cmd, "public", public),
				Versioning: optBool(cmd, "versioning", versioning),
				QuotaBytes: optInt(cmd, "quota-bytes", quota),
			}))
			if err != nil {
				return err
			}
			return env.Printer.Object(v, bucketColumns)
		},
	}
	create.Flags().BoolVar(&public, "public", false, "публичное чтение")
	create.Flags().BoolVar(&versioning, "versioning", false, "включить версионирование")
	create.Flags().IntVar(&quota, "quota-bytes", 0, "квота в байтах")

	var upublic, uversioning bool
	update := &cobra.Command{
		Use:         "update <бакет>",
		Short:       "Изменить бакет",
		Annotations: ops("object_storage_update_bucket"),
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, id, err := env.bucketTarget(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			v, err := call(c.ObjectStorageUpdateBucketWithResponse(cmd.Context(), id, tatnet.V1BucketUpdate{
				IsPublic:   optBool(cmd, "public", upublic),
				Versioning: optBool(cmd, "versioning", uversioning),
			}))
			if err != nil {
				return err
			}
			return env.Printer.Object(v, bucketColumns)
		},
	}
	update.Flags().BoolVar(&upublic, "public", false, "публичное чтение")
	update.Flags().BoolVar(&uversioning, "versioning", false, "версионирование")

	var yes, force bool
	del := &cobra.Command{
		Use:         "delete <бакет>",
		Short:       "Удалить бакет",
		Annotations: ops("object_storage_delete_bucket"),
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, id, err := env.bucketTarget(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			what := "Удалить бакет %s?"
			if force {
				what = "Удалить бакет %s ВМЕСТЕ С СОДЕРЖИМЫМ?"
			}
			if err := confirm(cmd, yes, what, args[0]); err != nil {
				return err
			}
			err = callOK(c.ObjectStorageDeleteBucketWithResponse(cmd.Context(), id,
				&tatnet.ObjectStorageDeleteBucketParams{Force: optBool(cmd, "force", force)}))
			if err != nil {
				return err
			}
			return env.Printer.Message("Бакет %s удалён.", args[0])
		},
	}
	addYes(del, &yes)
	del.Flags().BoolVar(&force, "force", false, "удалить вместе с объектами")

	cmd.AddCommand(list, get, create, update, del)
	return cmd
}

func s3ObjectCommand(env *Env) *cobra.Command {
	cmd := &cobra.Command{Use: "object", Aliases: []string{"objects"}, Short: "Объекты в бакете"}

	var prefix string
	var limit int
	list := &cobra.Command{
		Use:         "list <бакет>",
		Short:       "Объекты бакета",
		Annotations: ops("object_storage_list_objects"),
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, id, err := env.bucketTarget(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			// Свой цикл, а не paginate: у объектов страницы идут по
			// continuation-токену S3, а не по offset.
			var (
				objects []any
				folders []any
				token   *string
			)
			for {
				v, err := call(c.ObjectStorageListObjectsWithResponse(cmd.Context(), id,
					&tatnet.ObjectStorageListObjectsParams{
						Prefix:            optStr(cmd, "prefix", prefix),
						ContinuationToken: token,
					}))
				if err != nil {
					return err
				}
				m, _ := v.(map[string]any)
				if page, ok := m["objects"].([]any); ok {
					objects = append(objects, page...)
				}
				if page, ok := m["folders"].([]any); ok {
					folders = append(folders, page...)
				}
				next, _ := m["next_continuation_token"].(string)
				truncated, _ := m["is_truncated"].(bool)
				if !truncated || next == "" || (limit > 0 && len(objects) >= limit) {
					break
				}
				token = &next
			}
			if limit > 0 && len(objects) > limit {
				objects = objects[:limit]
			}
			if len(folders) > 0 && env.Printer.Format != output.JSON && env.Printer.Format != output.YAML {
				fmt.Fprintln(cmd.OutOrStdout(), "папки: "+output.Stringify(folders))
			}
			return env.Printer.List(objects, []output.Column{
				output.Col("ключ", "key"),
				output.Col("размер", "size"),
				output.Col("изменён", "last_modified"),
				output.WideCol("etag", "etag"),
			})
		},
	}
	list.Flags().StringVar(&prefix, "prefix", "", "только ключи с этим префиксом")
	list.Flags().IntVar(&limit, "limit", 0, "не больше стольких объектов (0 — все)")

	var yes bool
	del := &cobra.Command{
		Use:         "delete <бакет> <ключ>",
		Short:       "Удалить объект",
		Annotations: ops("object_storage_delete_object"),
		Args:        cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, id, err := env.bucketTarget(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if err := confirm(cmd, yes, "Удалить объект %s из %s?", args[1], args[0]); err != nil {
				return err
			}
			if err := callOK(c.ObjectStorageDeleteObjectWithResponse(cmd.Context(), id,
				tatnet.V1ObjectKey{Key: args[1]})); err != nil {
				return err
			}
			return env.Printer.Message("Объект %s удалён.", args[1])
		},
	}
	addYes(del, &yes)

	var method string
	presign := &cobra.Command{
		Use:   "presign <бакет> <ключ>",
		Short: "Временная ссылка на объект",
		Long: "Выдаёт подписанную ссылку на объект.\n\n" +
			"Ссылка даёт доступ всем, у кого она есть, до истечения срока —\n" +
			"это выданный доступ, а не просто адрес.",
		Annotations: ops("object_storage_presign_object"),
		Args:        cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, id, err := env.bucketTarget(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			v, err := call(c.ObjectStoragePresignObjectWithResponse(cmd.Context(), id, tatnet.V1Presign{
				Key: args[1], Method: optStr(cmd, "method", method),
			}))
			if err != nil {
				return err
			}
			return env.Printer.Object(v, []output.Column{
				output.Col("ссылка", "url"),
				output.Col("метод", "method"),
				output.Col("действует до", "expires_at"),
			})
		},
	}
	presign.Flags().StringVar(&method, "method", "", "метод: GET или PUT")

	folder := &cobra.Command{
		Use:         "folder <бакет> <префикс>",
		Short:       "Создать папку (пустой префикс)",
		Annotations: ops("object_storage_create_folder"),
		Args:        cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, id, err := env.bucketTarget(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			v, err := call(c.ObjectStorageCreateFolderWithResponse(cmd.Context(), id,
				tatnet.V1FolderCreate{Prefix: args[1]}))
			if err != nil {
				return err
			}
			return env.Printer.Object(v, []output.Column{output.Col("префикс", "prefix")})
		},
	}

	cmd.AddCommand(list, del, presign, folder)
	return cmd
}

func s3KeyCommand(env *Env) *cobra.Command {
	cmd := &cobra.Command{Use: "key", Aliases: []string{"keys", "access-key"}, Short: "Ключи доступа S3"}

	listKeys := func(ctx context.Context) ([]any, error) {
		c, err := env.Client()
		if err != nil {
			return nil, err
		}
		return paginate(ctx, func(ctx context.Context, offset, size int) (any, error) {
			return call(c.ObjectStorageListAccessKeysWithResponse(ctx,
				&tatnet.ObjectStorageListAccessKeysParams{Limit: &size, Offset: &offset}))
		}, 0, 0)
	}

	list := &cobra.Command{
		Use:         "list",
		Short:       "Список ключей",
		Annotations: ops("object_storage_list_access_keys"),
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			all, err := listKeys(cmd.Context())
			if err != nil {
				return err
			}
			return env.Printer.List(all, accessKeyColumns)
		},
	}

	var buckets []string
	create := &cobra.Command{
		Use:   "create <имя>",
		Short: "Создать ключ доступа",
		Long: "Создаёт пару ключей SigV4 для доступа к бакетам.\n\n" +
			"Секрет показывается ОДИН раз — в ответе на создание. Позже его\n" +
			"получить нельзя, потерянный ключ только пересоздаётся.\n\n" +
			"Без --bucket ключ получает доступ ко всем бакетам аккаунта,\n" +
			"включая будущие.",
		Annotations: ops("object_storage_create_access_key"),
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := env.Client()
			if err != nil {
				return err
			}
			body := tatnet.V1AccessKeyCreate{Name: args[0], AllBuckets: len(buckets) == 0}
			if len(buckets) > 0 {
				ids := make([]string, 0, len(buckets))
				for _, b := range buckets {
					id, err := resolveRef(cmd.Context(), "бакет", b, env.listBuckets, "name")
					if err != nil {
						return err
					}
					ids = append(ids, id)
				}
				body.BucketIds = &ids
			}
			v, err := call(c.ObjectStorageCreateAccessKeyWithResponse(cmd.Context(), body))
			if err != nil {
				return err
			}
			return env.Printer.Object(v, append([]output.Column{
				output.Col("секрет (больше не покажем)", "secret_access_key"),
			}, accessKeyColumns...))
		},
	}
	create.Flags().StringSliceVar(&buckets, "bucket", nil, "ограничить ключ этими бакетами (можно повторять)")

	var ubuckets []string
	var allBuckets bool
	update := &cobra.Command{
		Use:         "update <ключ>",
		Short:       "Изменить область действия ключа",
		Annotations: ops("object_storage_update_access_key"),
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := env.Client()
			if err != nil {
				return err
			}
			id, err := resolveRef(cmd.Context(), "ключ доступа", args[0], listKeys, "name", "access_key_id")
			if err != nil {
				return err
			}
			body := tatnet.V1AccessKeyScope{AllBuckets: allBuckets}
			if len(ubuckets) > 0 {
				ids := make([]string, 0, len(ubuckets))
				for _, b := range ubuckets {
					bid, err := resolveRef(cmd.Context(), "бакет", b, env.listBuckets, "name")
					if err != nil {
						return err
					}
					ids = append(ids, bid)
				}
				body.BucketIds = &ids
			}
			v, err := call(c.ObjectStorageUpdateAccessKeyWithResponse(cmd.Context(), id, body))
			if err != nil {
				return err
			}
			return env.Printer.Object(v, accessKeyColumns)
		},
	}
	update.Flags().StringSliceVar(&ubuckets, "bucket", nil, "разрешённые бакеты")
	update.Flags().BoolVar(&allBuckets, "all-buckets", false, "доступ ко всем бакетам")

	var yes bool
	del := &cobra.Command{
		Use:         "delete <ключ>",
		Short:       "Удалить ключ доступа",
		Annotations: ops("object_storage_delete_access_key"),
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := env.Client()
			if err != nil {
				return err
			}
			id, err := resolveRef(cmd.Context(), "ключ доступа", args[0], listKeys, "name", "access_key_id")
			if err != nil {
				return err
			}
			if err := confirm(cmd, yes, "Удалить ключ доступа %s? Всё, что им пользуется, перестанет работать.", args[0]); err != nil {
				return err
			}
			if err := callOK(c.ObjectStorageDeleteAccessKeyWithResponse(cmd.Context(), id)); err != nil {
				return err
			}
			return env.Printer.Message("Ключ %s удалён.", args[0])
		},
	}
	addYes(del, &yes)

	cmd.AddCommand(list, create, update, del)
	return cmd
}
