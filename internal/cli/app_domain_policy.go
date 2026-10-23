package cli

import (
	"encoding/json"
	"fmt"
	"github.com/spf13/cobra"
	"net/url"
)

// The refreshed SDK contract also exposes the existing domain policy API.
func appDomainPolicyCommand(env *Env) *cobra.Command {
	parent := &cobra.Command{Use: "domain-policy", Short: "Политика технического домена приложения"}
	for _, action := range []string{"get", "set", "restore"} {
		var mode, primary, domain string
		op := map[string]string{"get": "apps_get_domain_policy", "set": "apps_set_domain_policy", "restore": "apps_restore_technical_domain"}[action]
		cmd := &cobra.Command{Use: action + " <app>", Short: map[string]string{"get": "Показать политику", "set": "Изменить политику", "restore": "Восстановить технический домен"}[action], Annotations: ops(op), Args: cobra.ExactArgs(1)}
		cmd.RunE = func(cmd *cobra.Command, args []string) error {
			_, project, id, err := env.appTarget(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			path := "/projects/" + url.PathEscape(project) + "/apps/" + url.PathEscape(id) + "/domain-policy"
			method := "GET"
			var body []byte
			if action == "set" {
				if mode != "serve" && mode != "redirect" && mode != "noindex" {
					return fmt.Errorf("--mode: serve или redirect")
				}
				method = "PATCH"
				payload := map[string]any{"mode": mode}
				if primary != "" {
					payload["primary_domain_id"] = primary
				}
				body, _ = json.Marshal(payload)
			}
			if action == "restore" {
				method = "POST"
				path += "/restore"
				body, _ = json.Marshal(map[string]string{"domain": domain})
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
		if action == "set" {
			cmd.Flags().StringVar(&mode, "mode", "", "serve или redirect")
			cmd.Flags().StringVar(&primary, "primary-domain-id", "", "UUID основного домена")
			_ = cmd.MarkFlagRequired("mode")
		}
		if action == "restore" {
			cmd.Flags().StringVar(&domain, "domain", "", "зарезервированный технический домен")
			_ = cmd.MarkFlagRequired("domain")
		}
		parent.AddCommand(cmd)
	}
	return parent
}
