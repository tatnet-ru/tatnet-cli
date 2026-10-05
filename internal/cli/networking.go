package cli

import (
	"context"
	"github.com/spf13/cobra"
	"github.com/tatnet-ru/tatnet-cli/internal/output"
	"github.com/tatnet-ru/tatnet-go/tatnet"
)

var vpcColumns = []output.Column{output.Col("имя", "name"), output.Col("id", "id"), output.Col("регион", "cluster_id"), output.Col("подсеть", "subnet"), output.Col("статус", "status"), output.Col("NAT", "nat_gateway.status"), output.WideCol("адрес NAT", "nat_gateway.address")}
var fipColumns = []output.Column{output.Col("имя", "name"), output.Col("id", "id"), output.Col("адрес", "address"), output.Col("регион", "cluster_id"), output.Col("статус", "status"), output.Col("интерфейс VM", "vm_interface_id"), output.WideCol("ошибка", "last_error")}

func (e *Env) listVpcs(ctx context.Context, cluster string) ([]any, error) {
	c, err := e.Client()
	if err != nil {
		return nil, err
	}
	return paginate(ctx, func(ctx context.Context, offset, size int) (any, error) {
		p := &tatnet.NetworkingListVpcsParams{Limit: &size, Offset: &offset}
		if cluster != "" {
			p.ClusterId = &cluster
		}
		return call(c.NetworkingListVpcsWithResponse(ctx, p))
	}, 0, 0)
}
func (e *Env) vpcID(ctx context.Context, ref string) (string, error) {
	return resolveRef(ctx, refKind{"сеть", "не найдена", "сетей"}, ref, func(ctx context.Context) ([]any, error) { return e.listVpcs(ctx, "") }, "name")
}

func networkCommand(e *Env, use, short, op string, args int, cols []output.Column, run func(*cobra.Command, *tatnet.ClientWithResponses, []string) (any, error)) *cobra.Command {
	return &cobra.Command{Use: use, Short: short, Annotations: ops(op), Args: cobra.ExactArgs(args), RunE: func(cmd *cobra.Command, a []string) error {
		c, err := e.Client()
		if err != nil {
			return err
		}
		v, err := run(cmd, c, a)
		if err != nil {
			return err
		}
		return e.Printer.Object(v, cols)
	}}
}

func newVPCCommand(e *Env) *cobra.Command {
	root := &cobra.Command{Use: "vpc", Aliases: []string{"vpcs"}, Short: "Приватные сети аккаунта"}
	var cluster, name, subnet string
	list := &cobra.Command{Use: "list", Short: "Сети аккаунта", Annotations: ops("networking_list_vpcs"), Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		v, err := e.listVpcs(cmd.Context(), cluster)
		if err != nil {
			return err
		}
		return e.Printer.List(v, vpcColumns)
	}}
	list.Flags().StringVar(&cluster, "cluster", "", "фильтр по id региона")
	create := networkCommand(e, "create", "Создать сеть", "networking_create_vpc", 0, vpcColumns, func(cmd *cobra.Command, c *tatnet.ClientWithResponses, _ []string) (any, error) {
		return call(c.NetworkingCreateVpcWithResponse(cmd.Context(), tatnet.V1VpcCreate{Name: name, Subnet: subnet, ClusterId: cluster}))
	})
	create.Flags().StringVar(&name, "name", "", "имя сети")
	create.Flags().StringVar(&subnet, "subnet", "", "CIDR приватной подсети")
	create.Flags().StringVar(&cluster, "cluster", "", "id региона")
	for _, f := range []string{"name", "subnet", "cluster"} {
		_ = create.MarkFlagRequired(f)
	}
	get := networkCommand(e, "get <сеть>", "Информация о сети", "networking_get_vpc", 1, vpcColumns, func(cmd *cobra.Command, c *tatnet.ClientWithResponses, a []string) (any, error) {
		id, err := e.vpcID(cmd.Context(), a[0])
		if err != nil {
			return nil, err
		}
		return call(c.NetworkingGetVpcWithResponse(cmd.Context(), id))
	})
	var yes bool
	del := &cobra.Command{Use: "delete <сеть>", Short: "Удалить сеть", Annotations: ops("networking_delete_vpc"), Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, a []string) error {
		id, err := e.vpcID(cmd.Context(), a[0])
		if err != nil {
			return err
		}
		if err = confirm(cmd, yes, "Удалить сеть %s?", a[0]); err != nil {
			return err
		}
		c, err := e.Client()
		if err != nil {
			return err
		}
		if err = callOK(c.NetworkingDeleteVpcWithResponse(cmd.Context(), id)); err != nil {
			return err
		}
		return e.Printer.Message("Сеть %s удалена.", a[0])
	}}
	addYes(del, &yes)
	nat := &cobra.Command{Use: "nat", Short: "NAT-шлюз сети"}
	var fip string
	var disableYes bool
	enable := networkCommand(e, "enable <сеть>", "Включить NAT", "networking_enable_nat_gateway", 1, []output.Column{output.Col("статус", "status"), output.Col("адрес", "address"), output.Col("fip", "fip_id")}, func(cmd *cobra.Command, c *tatnet.ClientWithResponses, a []string) (any, error) {
		id, err := e.vpcID(cmd.Context(), a[0])
		if err != nil {
			return nil, err
		}
		return call(c.NetworkingEnableNatGatewayWithResponse(cmd.Context(), id, tatnet.V1NatGatewayEnable{FipId: optStr(cmd, "floating-ip", fip)}))
	})
	enable.Flags().StringVar(&fip, "floating-ip", "", "id существующего свободного floating IP (иначе выделяется новый)")
	disable := &cobra.Command{Use: "disable <сеть>", Short: "Выключить NAT", Annotations: ops("networking_disable_nat_gateway"), Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, a []string) error {
		id, err := e.vpcID(cmd.Context(), a[0])
		if err != nil {
			return err
		}
		if err = confirm(cmd, disableYes, "Выключить NAT сети %s?", a[0]); err != nil {
			return err
		}
		c, err := e.Client()
		if err != nil {
			return err
		}
		if err = callOK(c.NetworkingDisableNatGatewayWithResponse(cmd.Context(), id)); err != nil {
			return err
		}
		return e.Printer.Message("NAT выключен.")
	}}
	addYes(disable, &disableYes)
	nat.AddCommand(enable, disable)
	reserved := &cobra.Command{Use: "reserved-ip", Short: "Резервирование адресов в сети"}
	var address, mac, label string
	var releaseYes bool
	reserve := networkCommand(e, "create <сеть>", "Зарезервировать адрес", "networking_create_reserved_ip", 1, []output.Column{output.Col("id", "id"), output.Col("имя", "name"), output.Col("адрес", "address"), output.Col("MAC", "mac")}, func(cmd *cobra.Command, c *tatnet.ClientWithResponses, a []string) (any, error) {
		id, err := e.vpcID(cmd.Context(), a[0])
		if err != nil {
			return nil, err
		}
		return call(c.NetworkingCreateReservedIpWithResponse(cmd.Context(), id, tatnet.V1ReservedIpCreate{Name: label, Address: address, Mac: mac}))
	})
	reserve.Flags().StringVar(&label, "name", "", "имя резерва")
	reserve.Flags().StringVar(&address, "address", "", "IP-адрес")
	reserve.Flags().StringVar(&mac, "mac", "", "MAC-адрес")
	for _, f := range []string{"name", "address", "mac"} {
		_ = reserve.MarkFlagRequired(f)
	}
	release := &cobra.Command{Use: "delete <сеть> <id-резерва>", Short: "Освободить адрес", Annotations: ops("networking_delete_reserved_ip"), Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, a []string) error {
		id, err := e.vpcID(cmd.Context(), a[0])
		if err != nil {
			return err
		}
		if err = confirm(cmd, releaseYes, "Освободить адрес %s?", a[1]); err != nil {
			return err
		}
		c, err := e.Client()
		if err != nil {
			return err
		}
		if err = callOK(c.NetworkingDeleteReservedIpWithResponse(cmd.Context(), id, a[1])); err != nil {
			return err
		}
		return e.Printer.Message("Адрес освобождён.")
	}}
	addYes(release, &releaseYes)
	reserved.AddCommand(reserve, release)
	root.AddCommand(list, create, get, del, nat, reserved)
	return root
}

func newFloatingIPCommand(e *Env) *cobra.Command {
	root := &cobra.Command{Use: "floating-ip", Aliases: []string{"floating-ips"}, Short: "Публичные плавающие IP аккаунта"}
	listIPs := func(ctx context.Context) ([]any, error) {
		c, err := e.Client()
		if err != nil {
			return nil, err
		}
		return paginate(ctx, func(ctx context.Context, offset, size int) (any, error) {
			return call(c.NetworkingListFloatingIpsWithResponse(ctx, &tatnet.NetworkingListFloatingIpsParams{Limit: &size, Offset: &offset}))
		}, 0, 0)
	}
	resolve := func(ctx context.Context, ref string) (string, error) {
		return resolveRef(ctx, refKind{"адрес", "не найден", "адресов"}, ref, listIPs, "name", "address")
	}
	list := &cobra.Command{Use: "list", Short: "Список адресов", Annotations: ops("networking_list_floating_ips"), Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		v, err := listIPs(cmd.Context())
		if err != nil {
			return err
		}
		return e.Printer.List(v, fipColumns)
	}}
	var name, cluster, newName, iface string
	create := networkCommand(e, "create", "Выделить адрес", "networking_create_floating_ip", 0, fipColumns, func(cmd *cobra.Command, c *tatnet.ClientWithResponses, _ []string) (any, error) {
		return call(c.NetworkingCreateFloatingIpWithResponse(cmd.Context(), tatnet.V1FloatingIPCreate{ClusterId: cluster, Name: optStr(cmd, "name", name)}))
	})
	create.Flags().StringVar(&name, "name", "", "имя")
	create.Flags().StringVar(&cluster, "cluster", "", "id региона")
	_ = create.MarkFlagRequired("cluster")
	get := networkCommand(e, "get <адрес>", "Информация об адресе", "networking_get_floating_ip", 1, fipColumns, func(cmd *cobra.Command, c *tatnet.ClientWithResponses, a []string) (any, error) {
		id, err := resolve(cmd.Context(), a[0])
		if err != nil {
			return nil, err
		}
		return call(c.NetworkingGetFloatingIpWithResponse(cmd.Context(), id))
	})
	update := networkCommand(e, "update <адрес>", "Изменить имя адреса", "networking_update_floating_ip", 1, fipColumns, func(cmd *cobra.Command, c *tatnet.ClientWithResponses, a []string) (any, error) {
		id, err := resolve(cmd.Context(), a[0])
		if err != nil {
			return nil, err
		}
		return call(c.NetworkingUpdateFloatingIpWithResponse(cmd.Context(), id, tatnet.V1FloatingIPUpdate{Name: &newName}))
	})
	update.Flags().StringVar(&newName, "name", "", "новое имя")
	_ = update.MarkFlagRequired("name")
	attach := networkCommand(e, "attach <адрес>", "Привязать к интерфейсу VM", "networking_attach_floating_ip", 1, fipColumns, func(cmd *cobra.Command, c *tatnet.ClientWithResponses, a []string) (any, error) {
		id, err := resolve(cmd.Context(), a[0])
		if err != nil {
			return nil, err
		}
		return call(c.NetworkingAttachFloatingIpWithResponse(cmd.Context(), id, tatnet.V1FloatingIPAttach{VmInterfaceId: iface}))
	})
	attach.Flags().StringVar(&iface, "interface", "", "UUID интерфейса VM в VPC")
	_ = attach.MarkFlagRequired("interface")
	root.AddCommand(list, create, get, update, attach)
	for _, action := range []string{"delete", "detach"} {
		var yes bool
		op := "networking_delete_floating_ip"
		short := "Освободить адрес"
		if action == "detach" {
			op = "networking_detach_floating_ip"
			short = "Отвязать адрес от VM"
		}
		cmd := &cobra.Command{Use: action + " <адрес>", Short: short, Annotations: ops(op), Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, a []string) error {
			id, err := resolve(cmd.Context(), a[0])
			if err != nil {
				return err
			}
			if err = confirm(cmd, yes, "%s %s?", short, a[0]); err != nil {
				return err
			}
			c, err := e.Client()
			if err != nil {
				return err
			}
			if action == "delete" {
				err = callOK(c.NetworkingDeleteFloatingIpWithResponse(cmd.Context(), id))
			} else {
				err = callOK(c.NetworkingDetachFloatingIpWithResponse(cmd.Context(), id))
			}
			if err != nil {
				return err
			}
			return e.Printer.Message("Операция выполнена.")
		}}
		addYes(cmd, &yes)
		root.AddCommand(cmd)
	}
	return root
}
