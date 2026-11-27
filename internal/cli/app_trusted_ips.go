package cli

import (
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

func appTrustedIPsCommand(env *Env) *cobra.Command {
	parent := &cobra.Command{
		Use:   "trusted-ips",
		Short: "Ограничить доступ к приложению по IP",
	}
	get := &cobra.Command{
		Use:         "get <app>",
		Short:       "Показать разрешённые IP",
		Annotations: ops("apps_get_app_trusted_ips"),
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAppTrustedIPs(cmd, env, args[0], http.MethodGet, nil)
		},
	}
	set := &cobra.Command{
		Use:         "set <app> <IPv4-or-CIDR>...",
		Short:       "Заменить список разрешённых IP",
		Annotations: ops("apps_set_app_trusted_ips"),
		Args:        cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAppTrustedIPs(cmd, env, args[0], http.MethodPut, args[1:])
		},
	}
	clear := &cobra.Command{
		Use:   "clear <app>",
		Short: "Открыть доступ к приложению с любого IP",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAppTrustedIPs(cmd, env, args[0], http.MethodPut, []string{})
		},
	}
	parent.AddCommand(get, set, clear)
	return parent
}

func runAppTrustedIPs(cmd *cobra.Command, env *Env, app, method string, ips []string) error {
	_, project, id, err := env.appTarget(cmd.Context(), app)
	if err != nil {
		return err
	}
	path := "/projects/" + url.PathEscape(project) + "/apps/" + url.PathEscape(id) + "/trusted-ips"
	var body []byte
	if method == http.MethodPut {
		body, err = json.Marshal(map[string][]string{"trusted_ips": ips})
		if err != nil {
			return err
		}
	}
	status, _, data, err := env.rawRequest(cmd, method, path, body, nil)
	if err != nil {
		return err
	}
	if status >= 400 {
		return &APIError{Status: status, Detail: detailOf(data), Body: data}
	}
	return printRaw(cmd, env, data)
}
