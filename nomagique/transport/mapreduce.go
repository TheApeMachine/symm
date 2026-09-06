package transport

import "github.com/theapemachine/symm/nomagique/core"

// NewMapReduce composes the existing map and pipe transports. It contains no
// second scheduler, reduction implementation, queue, or callback protocol.
func NewMapReduce(mapping, reduction core.Primitive) core.Primitive {
	return NewPipe(NewMap(mapping), reduction)
}
