package cli

import (
	"encoding/json"
	"fmt"
	"github.com/spf13/cobra"
	"io"
	"net/http"
	"net/url"
	"os"
)

// Keep newly exported public API operations available when the SDK contract moves.
func requestRawJSON(env *Env, cmd *cobra.Command, method, path string, body []byte) error {
	status, _, raw, err := env.rawRequest(cmd, method, path, body, nil)
	if err != nil {
		return err
	}
	if status >= 400 {
		return &APIError{Status: status, Detail: detailOf(raw), Body: raw}
	}
	if status == http.StatusNoContent {
		return nil
	}
	return printRaw(cmd, env, raw)
}
func inputJSON(cmd *cobra.Command, path string) ([]byte, error) {
	var raw []byte
	var err error
	if path == "-" {
		raw, err = io.ReadAll(cmd.InOrStdin())
	} else {
		raw, err = os.ReadFile(path)
	}
	if err != nil {
		return nil, err
	}
	if !json.Valid(raw) {
		return nil, fmt.Errorf("ожидается JSON в --input")
	}
	return raw, nil
}
func newAccountFundsCommand(env *Env) *cobra.Command {
	var amount, period string
	cmd := &cobra.Command{Use: "funds", Short: "Проверить возможность оплаты и распределение денег/бонусов", Annotations: ops("account_preview_funds"), Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		q := url.Values{"amount": {amount}}
		if period != "" {
			q.Set("period_type", period)
		}
		return requestRawJSON(env, cmd, "GET", "/account/funds?"+q.Encode(), nil)
	}}
	cmd.Flags().StringVar(&amount, "amount", "", "Сумма для проверки, в рублях")
	cmd.Flags().StringVar(&period, "period", "", "Тип периода оплаты")
	_ = cmd.MarkFlagRequired("amount")
	return cmd
}
func newProjectImagesCommand(env *Env) *cobra.Command {
	var region string
	cmd := &cobra.Command{Use: "project-images", Short: "Образы, доступные аккаунту выбранного проекта", Annotations: ops("vms_list_images"), Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		project, err := env.RequireProject(cmd.Context())
		if err != nil {
			return err
		}
		path := "/projects/" + url.PathEscape(project) + "/images"
		if region != "" {
			path += "?" + url.Values{"cluster_id": {region}}.Encode()
		}
		return requestRawJSON(env, cmd, "GET", path, nil)
	}}
	cmd.Flags().StringVar(&region, "region", "", "Фильтр по региону")
	return cmd
}
func newDNSRRsetCommand(env *Env) *cobra.Command {
	root := &cobra.Command{Use: "rrset", Short: "Атомарные DNS RRsets с проверкой revision"}
	path := func(args []string) string {
		p := "/dns/zones/" + url.PathEscape(args[0]) + "/rrsets"
		if len(args) > 1 {
			p += "/" + url.PathEscape(args[1])
		}
		return p
	}
	get := &cobra.Command{Use: "get <zone-id> <rrset-id>", Args: cobra.ExactArgs(2), Annotations: ops("dns_get_dns_rrset"), RunE: func(cmd *cobra.Command, args []string) error { return requestRawJSON(env, cmd, "GET", path(args), nil) }}
	root.AddCommand(get)
	for _, spec := range []struct {
		name, method, op string
		args             int
	}{{"create", "POST", "dns_create_dns_rrset", 1}, {"replace", "PUT", "dns_replace_dns_rrset", 2}} {
		var input string
		use := spec.name + " <zone-id>"
		if spec.args == 2 {
			use += " <rrset-id>"
		}
		cmd := &cobra.Command{Use: use, Short: "JSON RRset из --input (для replace обязательна expected_revision)", Args: cobra.ExactArgs(spec.args), Annotations: ops(spec.op), RunE: func(cmd *cobra.Command, args []string) error {
			raw, err := inputJSON(cmd, input)
			if err != nil {
				return err
			}
			return requestRawJSON(env, cmd, spec.method, path(args), raw)
		}}
		cmd.Flags().StringVar(&input, "input", "", "JSON-файл или - для stdin")
		_ = cmd.MarkFlagRequired("input")
		root.AddCommand(cmd)
	}
	var revision string
	del := &cobra.Command{Use: "delete <zone-id> <rrset-id>", Short: "Удалить RRset только при совпадении revision", Args: cobra.ExactArgs(2), Annotations: ops("dns_delete_dns_rrset"), RunE: func(cmd *cobra.Command, args []string) error {
		return requestRawJSON(env, cmd, "DELETE", path(args)+"?"+url.Values{"expected_revision": {revision}}.Encode(), nil)
	}}
	del.Flags().StringVar(&revision, "revision", "", "Ожидаемая revision из get")
	_ = del.MarkFlagRequired("revision")
	root.AddCommand(del)
	return root
}
