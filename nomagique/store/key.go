package store

import (
	"encoding/json"
	"strconv"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
NewKey creates a generic Value closure that navigates nested data by path segments.
It extracts numerical or temporal values and converts them to *float64.
If not found or not a valid number, it returns nil.
*/
type Key[T any] types.Value[T, *float64]
func NewKey[T any](interest ...types.String) Key[T] {
	return func(in T) *float64 {
		var current any = in

		if current == nil || len(interest) == 0 {
			return nil
		}

		segments := make([]string, len(interest))
		for i, s := range interest {
			if s != nil {
				segments[i] = s(in)
			}
		}

		if rawBytes, ok := current.([]byte); ok {
			var decoded map[string]any
			if err := sonic.Unmarshal(rawBytes, &decoded); err == nil {
				current = decoded
			}
		}

		for _, segment := range segments {
			if current == nil {
				return nil
			}

			switch container := current.(type) {
			case map[string]any:
				val, ok := container[segment]
				if !ok {
					return nil
				}
				current = val

			case []any:
				if len(container) == 0 {
					return nil
				}
				if firstMap, ok := container[0].(map[string]any); ok {
					val, ok := firstMap[segment]
					if !ok {
						return nil
					}
					current = val
				} else {
					return nil
				}

			case []map[string]any:
				if len(container) == 0 {
					return nil
				}
				val, ok := container[0][segment]
				if !ok {
					return nil
				}
				current = val

			default:
				return nil
			}
		}

		if current == nil {
			return nil
		}

		lastSegment := segments[len(segments)-1]

		if lastSegment == "timestamp" {
			switch val := current.(type) {
			case int64:
				f := float64(val)
				return &f
			case float64:
				return &val
			case json.Number:
				if i, err := val.Int64(); err == nil {
					f := float64(i)
					return &f
				}
				if f, err := val.Float64(); err == nil {
					return &f
				}
			case string:
				if parsedTime, err := time.Parse(time.RFC3339Nano, val); err == nil {
					f := float64(parsedTime.UnixNano())
					return &f
				}
				if parsedTime, err := time.Parse(time.RFC3339, val); err == nil {
					f := float64(parsedTime.UnixNano())
					return &f
				}
				if i, err := strconv.ParseInt(val, 10, 64); err == nil {
					f := float64(i)
					return &f
				}
			}
		}

		switch val := current.(type) {
		case float64:
			return &val
		case float32:
			f := float64(val)
			return &f
		case int:
			f := float64(val)
			return &f
		case int64:
			f := float64(val)
			return &f
		case json.Number:
			if f, err := val.Float64(); err == nil {
				return &f
			}
		case string:
			if f, err := strconv.ParseFloat(val, 64); err == nil {
				return &f
			}
		}

		return nil
	}
}
