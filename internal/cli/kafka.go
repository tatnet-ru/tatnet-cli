package cli

import (
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

// Kafka uses the common authenticated /v1 transport; validation and business
// logic stay in the API, just as for the generated SDK-backed commands.
func newKafkaCommand(env *Env) *cobra.Command {
	root := &cobra.Command{Use: "kafka", Short: "Managed Kafka: Dev и HA, подключение из интернета и VPC"}
	request := func(cmd *cobra.Command, method, path string, body any) error {
		var data []byte
		var err error
		if body != nil {
			data, err = json.Marshal(body)
			if err != nil {
				return err
			}
		}
		status, _, response, err := env.rawRequest(cmd, method, path, data, nil)
		if err != nil {
			return err
		}
		if status >= 400 {
			return &APIError{Status: status, Detail: detailOf(response), Body: response}
		}
		return printRaw(cmd, env, response)
	}
	path := func(cmd *cobra.Command, id string) (string, error) {
		project, err := env.RequireProject(cmd.Context())
		if err != nil {
			return "", err
		}
		p := "/projects/" + url.PathEscape(project) + "/kafka-clusters"
		if id != "" {
			p += "/" + url.PathEscape(id)
		}
		return p, nil
	}
	root.AddCommand(&cobra.Command{Use: "regions", Short: "Регионы, версии и тарифы", Args: cobra.NoArgs, Annotations: ops("kafka_regions"), RunE: func(cmd *cobra.Command, _ []string) error { return request(cmd, http.MethodGet, "/kafka/regions", nil) }})
	root.AddCommand(&cobra.Command{Use: "list", Short: "Кластеры выбранного проекта", Args: cobra.NoArgs, Annotations: ops("kafka_list_clusters"), RunE: func(cmd *cobra.Command, _ []string) error {
		p, err := path(cmd, "")
		if err != nil {
			return err
		}
		return request(cmd, http.MethodGet, p, nil)
	}})
	for _, operation := range []struct{ use, description, method, id string }{{"get <id>", "Кластер, адреса подключения и CA", http.MethodGet, "kafka_get_cluster"}, {"delete <id>", "Удалить Kafka-кластер и его данные", http.MethodDelete, "kafka_delete_cluster"}} {
		root.AddCommand(&cobra.Command{Use: operation.use, Short: operation.description, Args: cobra.ExactArgs(1), Annotations: ops(operation.id), RunE: func(cmd *cobra.Command, args []string) error {
			p, err := path(cmd, args[0])
			if err != nil {
				return err
			}
			return request(cmd, operation.method, p, nil)
		}})
	}
	var name, region, plan, controller, version, topology, vpc string
	var allow []string
	var retention int
	create := &cobra.Command{Use: "create", Short: "Создать кластер (пароль выдаётся один раз)", Args: cobra.NoArgs, Annotations: ops("kafka_create_cluster"), RunE: func(cmd *cobra.Command, _ []string) error {
		p, err := path(cmd, "")
		if err != nil {
			return err
		}
		body := map[string]any{"name": name, "cluster_id": region, "plan_id": plan, "topology": topology, "ip_allowlist": allow, "retention_hours": retention}
		for key, value := range map[string]string{"controller_plan_id": controller, "kafka_version": version, "vpc_id": vpc} {
			if value != "" {
				body[key] = value
			}
		}
		return request(cmd, http.MethodPost, p, body)
	}}
	f := create.Flags()
	f.StringVar(&name, "name", "", "имя")
	f.StringVar(&region, "region", "", "id региона")
	f.StringVar(&plan, "plan", "", "id тарифа брокера")
	f.StringVar(&controller, "controller-plan", "", "id тарифа контроллеров HA")
	f.StringVar(&version, "version", "", "версия Kafka")
	f.StringVar(&topology, "topology", "dev", "dev или ha")
	f.StringVar(&vpc, "vpc", "", "id VPC для дополнительного приватного доступа")
	f.StringSliceVar(&allow, "allow", []string{}, "публичный список разрешённых IPv4 CIDR")
	f.IntVar(&retention, "retention-hours", 168, "хранение сообщений в часах")
	for _, flag := range []string{"name", "region", "plan"} {
		create.MarkFlagRequired(flag)
	}
	root.AddCommand(create)
	var cidrs []string
	access := &cobra.Command{Use: "allowlist <id>", Short: "Заменить публичный список разрешённых адресов", Args: cobra.ExactArgs(1), Annotations: ops("kafka_update_allowlist"), RunE: func(cmd *cobra.Command, args []string) error {
		p, err := path(cmd, args[0])
		if err != nil {
			return err
		}
		return request(cmd, http.MethodPut, p+"/allowlist", map[string]any{"ip_allowlist": cidrs})
	}}
	access.Flags().StringSliceVar(&cidrs, "allow", []string{}, "IPv4 CIDR; пусто — закрыть публичный доступ")
	root.AddCommand(access)
	root.AddCommand(&cobra.Command{Use: "resume <id>", Short: "Возобновить Kafka: оплатить час остановленных подписок и запустить узлы", Args: cobra.ExactArgs(1), Annotations: ops("kafka_resume_cluster"), RunE: func(cmd *cobra.Command, args []string) error {
		p, err := path(cmd, args[0])
		if err != nil {
			return err
		}
		return request(cmd, http.MethodPost, p+"/resume", nil)
	}})
	var power string
	canary := &cobra.Command{Use: "canary-power <cluster-id> <node-id>", Short: "Закрытый canary: start/stop узла; оплата продолжается", Args: cobra.ExactArgs(2), Annotations: ops("kafka_canary_node_action"), RunE: func(cmd *cobra.Command, args []string) error {
		p, err := path(cmd, args[0])
		if err != nil {
			return err
		}
		return request(cmd, http.MethodPost, p+"/nodes/"+url.PathEscape(args[1])+"/canary-power", map[string]string{"action": power})
	}}
	canary.Flags().StringVar(&power, "action", "", "start или stop")
	_ = canary.MarkFlagRequired("action")
	root.AddCommand(canary)

	return root
}
