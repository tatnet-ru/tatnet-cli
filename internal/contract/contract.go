// Package contract — вшитый контракт /v1.
//
// Нужен двум потребителям: команде `tatnet api`, которая проверяет адрес до
// отправки запроса, и тесту покрытия, который сверяет дерево команд с
// контрактом. Копия синхронизируется scripts/sync-contract.sh.
package contract

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
)

//go:embed v1.json
var Raw []byte

// Operation — одна операция контракта.
type Operation struct {
	ID      string
	Method  string
	Path    string
	Tag     string
	Summary string
}

var (
	once     sync.Once
	parsed   []Operation
	parseErr error
)

// Operations возвращает все операции контракта, отсортированные по адресу.
func Operations() ([]Operation, error) {
	once.Do(func() {
		var doc struct {
			Paths map[string]map[string]struct {
				OperationID string   `json:"operationId"`
				Summary     string   `json:"summary"`
				Tags        []string `json:"tags"`
			} `json:"paths"`
		}
		if err := json.Unmarshal(Raw, &doc); err != nil {
			parseErr = fmt.Errorf("вшитый контракт не разбирается: %w", err)
			return
		}
		for path, methods := range doc.Paths {
			for method, op := range methods {
				switch strings.ToUpper(method) {
				case "GET", "POST", "PUT", "PATCH", "DELETE":
				default:
					continue
				}
				tag := ""
				if len(op.Tags) > 0 {
					tag = op.Tags[0]
				}
				parsed = append(parsed, Operation{
					ID: op.OperationID, Method: strings.ToUpper(method),
					Path: path, Tag: tag, Summary: op.Summary,
				})
			}
		}
		sort.Slice(parsed, func(i, j int) bool {
			if parsed[i].Path != parsed[j].Path {
				return parsed[i].Path < parsed[j].Path
			}
			return parsed[i].Method < parsed[j].Method
		})
	})
	return parsed, parseErr
}

// Find ищет операцию по методу и конкретному адресу, подставляя шаблонные
// сегменты. Возвращает описание операции, если такой адрес в контракте есть.
func Find(method, path string) (Operation, bool) {
	ops, err := Operations()
	if err != nil {
		return Operation{}, false
	}
	method = strings.ToUpper(method)
	want := segments(path)
	for _, op := range ops {
		if op.Method != method {
			continue
		}
		if matchSegments(segments(op.Path), want) {
			return op, true
		}
	}
	return Operation{}, false
}

// Suggest возвращает похожие адреса — чтобы отказ называл, что рядом.
func Suggest(path string, limit int) []Operation {
	ops, err := Operations()
	if err != nil {
		return nil
	}
	want := segments(path)
	type scored struct {
		op    Operation
		score int
	}
	var all []scored
	for _, op := range ops {
		have := segments(op.Path)
		score := 0
		for i := 0; i < len(have) && i < len(want); i++ {
			switch {
			case have[i] == want[i]:
				score += 3
			case isPlaceholder(have[i]):
				score++
			case similar(have[i], want[i]):
				// Опечатка и усечение — самый частый способ промахнуться
				// мимо адреса, и подсказка нужна именно там: /vpc → /vpcs.
				score += 2
			}
		}
		if score > 0 {
			all = append(all, scored{op, score})
		}
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].score > all[j].score })
	out := make([]Operation, 0, limit)
	for _, s := range all {
		if len(out) >= limit {
			break
		}
		out = append(out, s.op)
	}
	return out
}

// similar — один сегмент является началом другого и они сопоставимой длины.
func similar(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	long, short := a, b
	if len(short) > len(long) {
		long, short = short, long
	}
	return len(short)*2 >= len(long) && strings.HasPrefix(long, short)
}

func segments(path string) []string {
	return strings.Split(strings.Trim(path, "/"), "/")
}

func isPlaceholder(s string) bool {
	return strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}")
}

func matchSegments(template, actual []string) bool {
	if len(template) != len(actual) {
		return false
	}
	for i := range template {
		if isPlaceholder(template[i]) {
			if actual[i] == "" {
				return false
			}
			continue
		}
		if template[i] != actual[i] {
			return false
		}
	}
	return true
}
