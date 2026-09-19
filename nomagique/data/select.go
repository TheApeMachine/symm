package data

import (
	"strings"

	"github.com/theapemachine/symm/nomagique/types"
)

/*
Select extracts nested fields or filters structured collections based on a key path.
No structs, pure Value closure.
*/
type Select types.Value[any, any]

func NewSelect(path string) Select {
	segments := strings.Split(path, ".")

	return func(in any) any {
		curr := in

		for _, segment := range segments {
			if curr == nil {
				return nil
			}

			asMap, ok := curr.(map[string]any)
			if !ok {
				return nil
			}

			curr = asMap[segment]
		}

		return curr
	}
}
