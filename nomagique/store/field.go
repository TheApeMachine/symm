package store

import (
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
Field yields the float64 carried by a keyed input when the key matches.
*/
type Field struct {
	*core.PrimitiveError

	key []string
	out float64
}

func NewField(key ...string) *Field {
	return &Field{PrimitiveError: core.NewPrimitiveError(), key: append([]string(nil), key...)}
}

func (field *Field) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			input := (*core.Input[string, []string, any])(arriving)

			if input == nil || input.Value == nil {
				continue
			}

			if len(input.Key) != len(field.key) {
				continue
			}

			matched := true

			for index := range field.key {
				if input.Key[index] != field.key[index] {
					matched = false
					break
				}
			}

			if !matched {
				continue
			}

			switch value := (*input.Value).(type) {
			case float64:
				field.out = value
			case int64:
				field.out = float64(value)
			default:
				continue
			}

			if !yield(unsafe.Pointer(&field.out)) {
				return
			}
		}
	}
}
