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

func NewSelect(path types.String) Select {
	return func(in any) any {
		p := ""
		if path != nil {
			p = path(in)
		}
		segments := strings.Split(p, ".")

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
