package hawkes

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique/types"
)

/*
Assemble gathers trade inputs into one Hawkes [2]float64 event:
[timestampNano, mark] where mark > 0 is buy and mark <= 0 is sell.
Pure Value closure taking port closures as constructor arguments.
*/
type Assemble types.Value[any, [2]float64]

func NewAssemble(
	timestamp types.Any,
	side types.String,
	symbol types.String,
) Assemble {
	return func(in any) [2]float64 {
		var ts float64
		var s string

		// 1. Evaluate timestamp port
		if timestamp != nil {
			if val := timestamp(in); val != nil {
				switch v := val.(type) {
				case float64:
					ts = v
				case int64:
					ts = float64(v)
				case int:
					ts = float64(v)
				case string:
					if m, ok := in.(map[string]any); ok {
						if tv, exists := m[v]; exists {
							if f, ok := tv.(float64); ok {
								ts = f
							} else if i, ok := tv.(int64); ok {
								ts = float64(i)
							}
						}
					}
				}
			}
		}

		// 2. Evaluate side port
		if side != nil {
			s = side(in)
			if (s == "side" || s == "") && in != nil {
				if m, ok := in.(map[string]any); ok {
					if sv, exists := m["side"]; exists {
						s = fmt.Sprint(sv)
					}
				}
			}
		}

		// 3. Direct extraction from frame map if ports didn't resolve value
		if ts == 0 || s == "" {
			if m, ok := in.(map[string]any); ok {
				if ts == 0 {
					if t, ok := m["timestamp"].(float64); ok {
						ts = t
					} else if t, ok := m["timestamp"].(int64); ok {
						ts = float64(t)
					} else if t, ok := m["time"].(float64); ok {
						ts = t
					} else if t, ok := m["time"].(int64); ok {
						ts = float64(t)
					}
				}
				if s == "" {
					if sv, ok := m["side"].(string); ok {
						s = sv
					}
				}
			}
		}

		mark := 1.0
		if s == "sell" || s == "s" {
			mark = -1.0
		}

		return [2]float64{ts, mark}
	}
}
