package ui

import (
	"maps"

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

func sanitizeWireMap(source map[string]any) (map[string]any, bool) {
	raw, err := sonic.Marshal(source)

	if err != nil {
		return nil, false
	}

	var out map[string]any

	if err := sonic.Unmarshal(raw, &out); err != nil {
		return nil, false
	}

	return out, true
}

func cloneWireMap(source map[string]any) map[string]any {
	return maps.Clone(source)
}
