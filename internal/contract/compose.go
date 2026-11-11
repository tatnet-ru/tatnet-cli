package contract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
)

// Compose preserves the published SDK contract and adds explicitly maintained
// raw-HTTP operations. An SDK release may include those operations already;
// conflicting definitions fail rather than silently overriding either contract.
func Compose(sdk, extension []byte) ([]byte, error) {
	decode := func(raw []byte) (map[string]any, error) {
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		var doc map[string]any
		err := decoder.Decode(&doc)
		if err == nil && doc == nil {
			err = fmt.Errorf("contract must be a JSON object")
		}
		return doc, err
	}
	doc, err := decode(sdk)
	if err != nil {
		return nil, err
	}
	extra, err := decode(extension)
	if err != nil {
		return nil, err
	}
	for key := range extra {
		if key != "paths" && key != "components" {
			return nil, fmt.Errorf("unsupported extension section %s", key)
		}
	}
	merge := func(section string, base, add any) error {
		existing, ok := base.(map[string]any)
		if !ok {
			return fmt.Errorf("missing SDK section %s", section)
		}
		additions, ok := add.(map[string]any)
		if !ok {
			return fmt.Errorf("invalid extension section %s", section)
		}
		for name, value := range additions {
			if old, found := existing[name]; found && !reflect.DeepEqual(old, value) {
				return fmt.Errorf("contract conflict at %s/%s", section, name)
			}
			existing[name] = value
		}
		return nil
	}
	if err := merge("paths", doc["paths"], extra["paths"]); err != nil {
		return nil, err
	}
	components, ok := doc["components"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("missing SDK components")
	}
	additions, ok := extra["components"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid extension components")
	}
	for key := range additions {
		if key != "schemas" {
			return nil, fmt.Errorf("unsupported extension components/%s", key)
		}
	}
	if err := merge("components/schemas", components["schemas"], additions["schemas"]); err != nil {
		return nil, err
	}
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(doc); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}
