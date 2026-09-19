package data

import "github.com/theapemachine/symm/nomagique/types"

type Extract types.Value[any, float64]

/*
NewExtract creates a pure closure that pulls a scalar value from structured data by key.
It recursively inspects maps, slices, and extracted pointers to locate the target numeric value.
*/
func NewExtract(path string) Extract {
	return func(in any) float64 {
		return extractFrom(in, path)
	}
}

func extractFrom(current any, key string) float64 {
	if current == nil {
		return 0
	}

	switch v := current.(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case *float64:
		if v != nil {
			return *v
		}
	case []*float64:
		if len(v) > 0 && v[0] != nil {
			return *v[0]
		}
	case map[string]any:
		if val, ok := v[key]; ok {
			return extractFrom(val, key)
		}
		for _, child := range v {
			if childMap, ok := child.(map[string]any); ok {
				if res := extractFrom(childMap, key); res != 0 {
					return res
				}
			} else if childSlice, ok := child.([]any); ok {
				if res := extractFrom(childSlice, key); res != 0 {
					return res
				}
			}
		}
	case []any:
		for _, item := range v {
			if res := extractFrom(item, key); res != 0 {
				return res
			}
		}
	}
	return 0
}
