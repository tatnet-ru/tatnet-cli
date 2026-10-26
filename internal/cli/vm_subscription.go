package cli

import (
	"fmt"
	"github.com/spf13/cobra"
	"github.com/tatnet-ru/tatnet-cli/internal/output"
	"github.com/tatnet-ru/tatnet-go/tatnet"
)

// The updated SDK includes the already-released subscription operations. Keep
// CLI coverage complete when syncing the shared public contract for Postgres.
func vmSubscriptionCommand(env *Env) *cobra.Command {
	columns := []output.Column{output.Col("период", "period_type"), output.Col("автопродление", "auto_renew"), output.Col("статус", "status"), output.Col("оплачено до", "paid_until")}
	cmd := &cobra.Command{Use: "subscription", Short: "Подписка ВМ"}
	get := &cobra.Command{Use: "get <ВМ>", Short: "Прочитать подписку ВМ", Annotations: ops("vms_get_vm_subscription_by_id"), Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		c, _, id, err := env.vmTarget(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		value, err := call(c.VmsGetVmSubscriptionByIdWithResponse(cmd.Context(), id))
		if err != nil {
			return err
		}
		return env.Printer.Object(value, columns)
	}}
	var period string
	var renew bool
	update := &cobra.Command{Use: "update <ВМ>", Short: "Сменить период или автопродление подписки ВМ", Annotations: ops("vms_patch_vm_subscription_by_id"), Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		body := tatnet.V1VMSubscriptionUpdate{}
		if cmd.Flags().Changed("period-type") {
			value := tatnet.V1VMSubscriptionUpdatePeriodType(period)
			if !value.Valid() {
				return fmt.Errorf("период: 1h, 1d, 1m, 3m, 6m или 12m")
			}
			body.PeriodType = &value
		}
		if cmd.Flags().Changed("auto-renew") {
			body.AutoRenew = &renew
		}
		if body.PeriodType == nil && body.AutoRenew == nil {
			return fmt.Errorf("укажите --period-type или --auto-renew")
		}
		c, _, id, err := env.vmTarget(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		value, err := call(c.VmsPatchVmSubscriptionByIdWithResponse(cmd.Context(), id, body))
		if err != nil {
			return err
		}
		return env.Printer.Object(value, columns)
	}}
	update.Flags().StringVar(&period, "period-type", "", "период: 1h, 1d, 1m, 3m, 6m, 12m")
	update.Flags().BoolVar(&renew, "auto-renew", false, "автопродление (для отключения --auto-renew=false)")
	cmd.AddCommand(get, update)
	return cmd
}
