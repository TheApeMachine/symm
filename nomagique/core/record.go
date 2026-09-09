package core

import (
	"fmt"
	"strings"
)

/* Record lifts named Go boundary values without introducing a second record type. */
func Record(values map[string]any) Primitive {
	fields := make(map[string]Primitive, len(values))
	for name, value := range values {
		fields[name] = From(value)
	}
	return From(fields)
}

/* Field decodes a required field at a Go boundary, distinguishing absence from zero. */
func Field[Value any](fields map[string]Primitive, path ...string) (Value, error) {
	var zero Value
	if len(path) == 0 {
		return zero, fmt.Errorf("primitive record: field path required")
	}
	for index, name := range path {
		field, exists := fields[name]
		if !exists || field == nil {
			return zero, fmt.Errorf("primitive record: required field %q is absent", strings.Join(path[:index+1], "."))
		}
		if index == len(path)-1 {
			value := To[Value](field)
			if err := field.Error(); err != nil {
				return zero, fmt.Errorf("primitive record %q: %w", strings.Join(path, "."), err)
			}
			return value, nil
		}
		fields = To[map[string]Primitive](field)
		if err := field.Error(); err != nil {
			return zero, fmt.Errorf("primitive record %q: %w", strings.Join(path[:index+1], "."), err)
		}
	}
	panic("unreachable")
}
