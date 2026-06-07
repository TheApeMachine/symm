package ui

import (
	"maps"
	"math"

	"github.com/bytedance/sonic"
)

/*
wireJSONObject normalizes a qpool payload into a JSON-safe map for websocket
fanout. Typed maps from producers (e.g. gauge frames with CategoryType values)
must round-trip through JSON so the hub never silently drops them on assert.
*/
func wireJSONObject(value any) (map[string]any, bool) {
	if out, ok := value.(map[string]any); ok {
		return sanitizeWireMap(out)
	}

	raw, err := sonic.Marshal(value)

	if err != nil {
		return nil, false
	}

	var out map[string]any

	if err := sonic.Unmarshal(raw, &out); err != nil {
		return nil, false
	}

	return out, true
}

/*
sanitizeWireMap normalizes a producer map into plain JSON types WITHOUT the
full marshal→unmarshal round trip the hub used to pay per frame: ~12 signals ×
gauge cadence made double-JSON the hub's hottest path. Plain values pass
through a type-switch walk; only exotic values (typed strings, structs, time)
fall back to per-VALUE JSON. Non-finite floats fail the frame, exactly as
sonic.Marshal did.
*/
func sanitizeWireMap(source map[string]any) (map[string]any, bool) {
	out := make(map[string]any, len(source))

	for key, value := range source {
		sanitized, ok := sanitizeWireValue(value)

		if !ok {
			return nil, false
		}

		out[key] = sanitized
	}

	return out, true
}

func sanitizeWireValue(value any) (any, bool) {
	switch typed := value.(type) {
	case nil, string, bool:
		return typed, true
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) {
			return nil, false
		}

		return typed, true
	case float32:
		return sanitizeWireValue(float64(typed))
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case uint64:
		return float64(typed), true
	case map[string]any:
		return sanitizeWireMap(typed)
	case []any:
		out := make([]any, len(typed))

		for index, element := range typed {
			sanitized, ok := sanitizeWireValue(element)

			if !ok {
				return nil, false
			}

			out[index] = sanitized
		}

		return out, true
	case []map[string]any:
		out := make([]any, len(typed))

		for index, element := range typed {
			sanitized, ok := sanitizeWireMap(element)

			if !ok {
				return nil, false
			}

			out[index] = sanitized
		}

		return out, true
	default:
		// Exotic value (typed string like CategoryType, struct, time.Time):
		// round-trip just THIS value through JSON, not the whole frame.
		raw, err := sonic.Marshal(typed)

		if err != nil {
			return nil, false
		}

		var generic any

		if err := sonic.Unmarshal(raw, &generic); err != nil {
			return nil, false
		}

		return generic, true
	}
}

func cloneWireMap(source map[string]any) map[string]any {
	return maps.Clone(source)
}
