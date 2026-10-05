package cli

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// Thin public inference API client. Validation, permissions, quotes and
// accounting remain in the gateway/API; tnai keys differ from platform keys.
func newAICommand(g *globalFlags) *cobra.Command {
	var key, base, kind, body string
	ai := &cobra.Command{Use: "ai", Short: "Чат, изображения и видео через ai.tatnet.cloud", PersistentPreRunE: func(*cobra.Command, []string) error { return nil }}
	ai.PersistentFlags().StringVar(&key, "inference-key", "", "ключ tnai_live_… (или $TATNET_INFERENCE_API_KEY)")
	ai.PersistentFlags().StringVar(&base, "inference-base-url", "https://ai.tatnet.cloud/v1", "адрес шлюза инференса")
	ai.PersistentFlags().StringVar(&kind, "kind", "image", "вид: chat, image или video")
	call := func(cmd *cobra.Command, method, path string, payload []byte, needKey bool) error {
		k := key
		if k == "" {
			k = os.Getenv("TATNET_INFERENCE_API_KEY")
		}
		if needKey && k == "" {
			return fmt.Errorf("нужен --inference-key или TATNET_INFERENCE_API_KEY")
		}
		req, err := http.NewRequestWithContext(cmd.Context(), method, strings.TrimRight(base, "/")+path, bytes.NewReader(payload))
		if err != nil {
			return err
		}
		if k != "" {
			req.Header.Set("Authorization", "Bearer "+k)
		}
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		client := &http.Client{Timeout: g.timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		raw, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
		if err != nil {
			return err
		}
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			return fmt.Errorf("инференс HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
		}
		_, err = cmd.OutOrStdout().Write(append(bytes.TrimSpace(raw), '\n'))
		return err
	}
	pathFor := func(operation string) (string, error) {
		switch kind {
		case "chat":
			if operation == "models" {
				return "/models", nil
			}
			if operation == "generate" {
				return "/chat/completions", nil
			}
		case "image":
			switch operation {
			case "models":
				return "/images/models", nil
			case "quote":
				return "/images/quote", nil
			case "generate":
				return "/images/jobs", nil
			}
		case "video":
			switch operation {
			case "models":
				return "/videos/models", nil
			case "quote":
				return "/videos/quote", nil
			case "generate":
				return "/videos", nil
			}
		}
		return "", fmt.Errorf("операция %s не поддерживает --kind %s", operation, kind)
	}
	models := &cobra.Command{Use: "models", Short: "Каталог и цены моделей", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		path, err := pathFor("models")
		if err != nil {
			return err
		}
		return call(cmd, "GET", path, nil, kind == "chat")
	}}
	ai.AddCommand(models)
	for _, operation := range []string{"quote", "generate"} {
		operation := operation
		c := &cobra.Command{Use: operation, Short: map[string]string{"quote": "Цена без запуска генерации", "generate": "Запустить генерацию (оплачивается по тарифу)"}[operation], Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := pathFor(operation)
			if err != nil {
				return err
			}
			payload := []byte(body)
			if strings.HasPrefix(body, "@") {
				payload, err = os.ReadFile(strings.TrimPrefix(body, "@"))
				if err != nil {
					return err
				}
			}
			if len(payload) == 0 {
				return fmt.Errorf("нужен --body JSON или --body @файл")
			}
			return call(cmd, "POST", path, payload, true)
		}}
		c.Flags().StringVar(&body, "body", "", "JSON запроса или @путь к файлу")
		ai.AddCommand(c)
	}
	ai.AddCommand(&cobra.Command{Use: "job ID", Short: "Статус и ссылки готовой задачи", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		prefix := "img_"
		path := "/images/jobs/"
		if kind == "video" {
			prefix = "vid_"
			path = "/videos/"
		} else if kind != "image" {
			return fmt.Errorf("job: kind image или video")
		}
		if !strings.HasPrefix(id, prefix) || strings.ContainsAny(id, "/?#") {
			return fmt.Errorf("неверный ID задачи")
		}
		return call(cmd, "GET", path+id, nil, true)
	}})
	return ai
}
