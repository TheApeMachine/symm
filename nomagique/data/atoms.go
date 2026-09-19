package data

import "github.com/theapemachine/symm/nomagique/types"

type Extract types.Value[map[string]any, float64]
/*
NewExtract creates a pure closure that pulls a scalar value from a map by key.
It completely avoids unsafe.Pointer casting and bespoke DTOs.
*/
func NewExtract(path string) Extract {
	return func(in map[string]any) float64 {
		if val, ok := in[path].(float64); ok {
			return val
		}
		return 0
	}
}
