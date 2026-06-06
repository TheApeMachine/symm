package types

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

/*
MarshalEnum renders a named enum value as its canonical lower-case name. The
reasoning subpackage reuses these helpers for its own enum tables (Unit, Condition,
Action, Subject, Comparison), so they live in this leaf package and are exported
rather than duplicated.
*/
func MarshalEnum[enumType comparable](value enumType, names map[enumType]string) (any, error) {
	name, ok := names[value]

	if ok {
		return name, nil
	}

	return nil, fmt.Errorf("perspectives: unknown enum value %v", value)
}

func MarshalEnumJSON[enumType comparable](value enumType, names map[enumType]string) ([]byte, error) {
	name, ok := names[value]

	if ok {
		return []byte(strconv.Quote(name)), nil
	}

	return nil, fmt.Errorf("perspectives: unknown enum value %v", value)
}

func UnmarshalEnum[enumType ~uint8](node *yaml.Node, target *enumType, names map[enumType]string) error {
	if node.Tag == "!!int" {
		return unmarshalNumericEnum(node, target, names)
	}

	value := normalizeEnumName(node.Value)

	for enumValue, name := range names {
		if normalizeEnumName(name) == value {
			*target = enumValue

			return nil
		}
	}

	return fmt.Errorf("perspectives: unknown enum value %q", node.Value)
}

func UnmarshalEnumJSON[enumType ~uint8](data []byte, target *enumType, names map[enumType]string) error {
	trimmed := strings.TrimSpace(string(data))

	if trimmed == "" {
		return fmt.Errorf("perspectives: empty enum value")
	}

	if trimmed[0] != '"' {
		parsed, err := strconv.ParseUint(trimmed, 10, 8)

		if err != nil {
			return err
		}

		return assignNumericEnum(enumType(parsed), target, names)
	}

	var name string

	if err := json.Unmarshal(data, &name); err != nil {
		return err
	}

	value := normalizeEnumName(name)

	for enumValue, enumName := range names {
		if normalizeEnumName(enumName) == value {
			*target = enumValue

			return nil
		}
	}

	return fmt.Errorf("perspectives: unknown enum value %q", name)
}

func unmarshalNumericEnum[enumType ~uint8](node *yaml.Node, target *enumType, names map[enumType]string) error {
	parsed, err := strconv.ParseUint(node.Value, 10, 8)

	if err != nil {
		return err
	}

	return assignNumericEnum(enumType(parsed), target, names)
}

func assignNumericEnum[enumType ~uint8](value enumType, target *enumType, names map[enumType]string) error {
	if _, ok := names[value]; !ok {
		return fmt.Errorf("perspectives: unknown enum value %d", value)
	}

	*target = value

	return nil
}

func normalizeEnumName(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ToLower(value)
	value = strings.ReplaceAll(value, "-", "_")

	return value
}
