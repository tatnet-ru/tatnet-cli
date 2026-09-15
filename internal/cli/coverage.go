package cli

// deferredTags — разделы контракта, для которых своих команд пока нет.
//
// Список существует ровно затем, чтобы непокрытое было видно. Операции этих
// разделов доступны через `tatnet api`, а тест покрытия следит за двумя
// вещами сразу: что покрытые разделы покрыты целиком и что отложенные не
// покрыты частично. Убрали раздел отсюда — обязаны завести команды на все
// его операции; завели команду на операцию отложенного раздела — обязаны
// убрать раздел.
var deferredTags = map[string]string{
	"load-balancers": "27 операций: слушатели, целевые группы, правила, сертификаты",
	"networking":     "VPC, плавающие адреса, NAT-шлюз, зарезервированные адреса",
	"kubernetes":     "кластеры, пулы узлов, kubeconfig, обновление",
	"functions":      "serverless-функции и их переменные",
	"volumes":        "сетевые диски",
	"domains":        "регистрация доменов: в /v1 только чтение",
	"certificates":   "одна операция чтения",
}

// CommandOps возвращает operationId, объявленные командой.
func CommandOps(annotations map[string]string) []string {
	raw, ok := annotations[opAnnotation]
	if !ok || raw == "" {
		return nil
	}
	return splitAndTrim(raw)
}

func splitAndTrim(s string) []string {
	var out []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ',' {
			if part := trimSpace(s[start:i]); part != "" {
				out = append(out, part)
			}
			start = i + 1
		}
	}
	return out
}

func trimSpace(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t' || s[0] == '\n') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t' || s[len(s)-1] == '\n') {
		s = s[:len(s)-1]
	}
	return s
}
