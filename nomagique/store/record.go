package store

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

// NewRecord composes field-producing expressions into KV. It introduces no
// record type and contains no arithmetic or observation interpretation.
func NewRecord(fields ...core.Primitive) core.Primitive {
	return transport.NewPipe(
		transport.NewFan(transport.NewPipe(), transport.NewIO(fields...)),
		NewKV[string](transport.NewIO(core.From(map[string]core.Primitive{}))),
	)
}
