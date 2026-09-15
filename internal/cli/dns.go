package cli

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/tatnet-ru/tatnet-go/tatnet"

	"github.com/tatnet-ru/tatnet-cli/internal/output"
)

var zoneColumns = []output.Column{
	output.Col("зона", "name"),
	output.Col("статус", "status"),
	output.Col("ttl", "default_ttl"),
	output.Col("dnssec", "dnssec_enabled"),
	output.WideCol("id", "id"),
	output.WideCol("создана", "created_at"),
}

var recordColumns = []output.Column{
	output.Col("имя", "name"),
	output.Col("тип", "type"),
	output.Col("значение", "content"),
	output.Col("ttl", "ttl"),
	output.Col("управляется", "managed"),
	output.WideCol("id", "id"),
}

func newDNSCommand(env *Env) *cobra.Command {
	cmd := &cobra.Command{Use: "dns", Short: "DNS-зоны и записи"}
	cmd.AddCommand(dnsZoneCommand(env), dnsRecordCommand(env))
	return cmd
}

func (e *Env) listZones(ctx context.Context) ([]any, error) {
	c, err := e.Client()
	if err != nil {
		return nil, err
	}
	return paginate(ctx, func(ctx context.Context, offset, size int) (any, error) {
		return call(c.DnsListZonesWithResponse(ctx, &tatnet.DnsListZonesParams{
			Limit: &size, Offset: &offset,
		}))
	}, 0, 0)
}

func (e *Env) resolveZone(ctx context.Context, ref string) (string, error) {
	return resolveRef(ctx, "зона", ref, e.listZones, "name")
}

func dnsZoneCommand(env *Env) *cobra.Command {
	cmd := &cobra.Command{Use: "zone", Aliases: []string{"zones"}, Short: "Зоны"}

	list := &cobra.Command{
		Use:         "list",
		Short:       "Список зон",
		Annotations: ops("dns_list_zones"),
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			all, err := env.listZones(cmd.Context())
			if err != nil {
				return err
			}
			return env.Printer.List(all, zoneColumns)
		},
	}

	get := &cobra.Command{
		Use:         "get <зона>",
		Short:       "Одна зона",
		Annotations: ops("dns_get_zone"),
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, id, err := env.zoneTarget(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			v, err := call(c.DnsGetZoneWithResponse(cmd.Context(), id))
			if err != nil {
				return err
			}
			return env.Printer.Object(v, zoneColumns)
		},
	}

	var ttl int
	var dnssec bool
	create := &cobra.Command{
		Use:         "create <домен>",
		Short:       "Создать зону",
		Annotations: ops("dns_create_zone"),
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := env.Client()
			if err != nil {
				return err
			}
			v, err := call(c.DnsCreateZoneWithResponse(cmd.Context(), tatnet.V1ZoneCreate{
				Name:          args[0],
				DefaultTtl:    optInt(cmd, "ttl", ttl),
				DnssecEnabled: optBool(cmd, "dnssec", dnssec),
			}))
			if err != nil {
				return err
			}
			return env.Printer.Object(v, zoneColumns)
		},
	}
	create.Flags().IntVar(&ttl, "ttl", 0, "TTL по умолчанию")
	create.Flags().BoolVar(&dnssec, "dnssec", false, "включить DNSSEC")

	var uttl int
	var udnssec bool
	update := &cobra.Command{
		Use:         "update <зона>",
		Short:       "Изменить зону",
		Annotations: ops("dns_update_zone"),
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, id, err := env.zoneTarget(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			v, err := call(c.DnsUpdateZoneWithResponse(cmd.Context(), id, tatnet.V1ZoneUpdate{
				DefaultTtl:    optInt(cmd, "ttl", uttl),
				DnssecEnabled: optBool(cmd, "dnssec", udnssec),
			}))
			if err != nil {
				return err
			}
			return env.Printer.Object(v, zoneColumns)
		},
	}
	update.Flags().IntVar(&uttl, "ttl", 0, "TTL по умолчанию")
	update.Flags().BoolVar(&udnssec, "dnssec", false, "включить DNSSEC")

	var yes bool
	del := &cobra.Command{
		Use:         "delete <зона>",
		Short:       "Удалить зону",
		Annotations: ops("dns_delete_zone"),
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, id, err := env.zoneTarget(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if err := confirm(cmd, yes, "Удалить зону %s со всеми записями?", args[0]); err != nil {
				return err
			}
			if err := callOK(c.DnsDeleteZoneWithResponse(cmd.Context(), id)); err != nil {
				return err
			}
			return env.Printer.Message("Зона %s удалена.", args[0])
		},
	}
	addYes(del, &yes)

	verify := &cobra.Command{
		Use:   "verify <зона>",
		Short: "Проверить делегирование NS",
		Long: "Проверяет, что домен делегирован на ns1/ns2.tatnet.ru.\n\n" +
			"До подтверждения делегирования выпуск сертификатов по DNS-01\n" +
			"для доменов этой зоны ждёт — проверка снимает ожидание.",
		Annotations: ops("dns_verify_zone"),
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, id, err := env.zoneTarget(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			v, err := call(c.DnsVerifyZoneWithResponse(cmd.Context(), id))
			if err != nil {
				return err
			}
			return env.Printer.Object(v, zoneColumns)
		},
	}

	cmd.AddCommand(list, get, create, update, del, verify)
	return cmd
}

func dnsRecordCommand(env *Env) *cobra.Command {
	cmd := &cobra.Command{Use: "record", Aliases: []string{"records"}, Short: "Записи зоны"}

	var nameFilter, typeFilter string
	list := &cobra.Command{
		Use:         "list <зона>",
		Short:       "Записи зоны",
		Annotations: ops("dns_list_dns_records"),
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, zone, err := env.zoneTarget(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			all, err := paginate(cmd.Context(), func(ctx context.Context, offset, size int) (any, error) {
				return call(c.DnsListDnsRecordsWithResponse(ctx, zone, &tatnet.DnsListDnsRecordsParams{
					Name:   optStr(cmd, "name", nameFilter),
					Type:   optStr(cmd, "type", typeFilter),
					Limit:  &size,
					Offset: &offset,
				}))
			}, 0, 0)
			if err != nil {
				return err
			}
			return env.Printer.List(all, recordColumns)
		},
	}
	list.Flags().StringVar(&nameFilter, "name", "", "только записи с этим именем")
	list.Flags().StringVar(&typeFilter, "type", "", "только записи этого типа")

	get := &cobra.Command{
		Use:         "get <зона> <запись-id>",
		Short:       "Одна запись",
		Annotations: ops("dns_get_dns_record"),
		Args:        cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, zone, err := env.zoneTarget(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			v, err := call(c.DnsGetDnsRecordWithResponse(cmd.Context(), zone, args[1]))
			if err != nil {
				return err
			}
			return env.Printer.Object(v, recordColumns)
		},
	}

	var rttl int
	create := &cobra.Command{
		Use:         "create <зона> <имя> <тип> <значение>",
		Short:       "Создать запись",
		Annotations: ops("dns_create_dns_record"),
		Args:        cobra.ExactArgs(4),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, zone, err := env.zoneTarget(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			v, err := call(c.DnsCreateDnsRecordWithResponse(cmd.Context(), zone, tatnet.V1DNSRecordCreate{
				Name: args[1], Type: args[2], Content: args[3],
				Ttl: optInt(cmd, "ttl", rttl),
			}))
			if err != nil {
				return err
			}
			return env.Printer.Object(v, recordColumns)
		},
	}
	create.Flags().IntVar(&rttl, "ttl", 0, "TTL записи")

	var uname, utype, ucontent string
	var uttl int
	update := &cobra.Command{
		Use:         "update <зона> <запись-id>",
		Short:       "Изменить запись",
		Annotations: ops("dns_update_dns_record"),
		Args:        cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, zone, err := env.zoneTarget(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			v, err := call(c.DnsUpdateDnsRecordWithResponse(cmd.Context(), zone, args[1], tatnet.V1DNSRecordUpdate{
				Name:    optStr(cmd, "name", uname),
				Type:    optStr(cmd, "type", utype),
				Content: optStr(cmd, "content", ucontent),
				Ttl:     optInt(cmd, "ttl", uttl),
			}))
			if err != nil {
				return err
			}
			return env.Printer.Object(v, recordColumns)
		},
	}
	update.Flags().StringVar(&uname, "name", "", "новое имя")
	update.Flags().StringVar(&utype, "type", "", "новый тип")
	update.Flags().StringVar(&ucontent, "content", "", "новое значение")
	update.Flags().IntVar(&uttl, "ttl", 0, "новый TTL")

	var yes bool
	del := &cobra.Command{
		Use:         "delete <зона> <запись-id>",
		Short:       "Удалить запись",
		Annotations: ops("dns_delete_dns_record"),
		Args:        cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, zone, err := env.zoneTarget(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if err := confirm(cmd, yes, "Удалить запись %s?", args[1]); err != nil {
				return err
			}
			if err := callOK(c.DnsDeleteDnsRecordWithResponse(cmd.Context(), zone, args[1])); err != nil {
				return err
			}
			return env.Printer.Message("Запись %s удалена.", args[1])
		},
	}
	addYes(del, &yes)

	cmd.AddCommand(list, get, create, update, del)
	return cmd
}

func (e *Env) zoneTarget(ctx context.Context, ref string) (*tatnet.ClientWithResponses, string, error) {
	c, err := e.Client()
	if err != nil {
		return nil, "", err
	}
	id, err := e.resolveZone(ctx, ref)
	if err != nil {
		return nil, "", err
	}
	return c, id, nil
}
