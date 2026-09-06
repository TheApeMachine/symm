package tests

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
	"testing"
)

// NewRun uses production delivery; tests do not maintain a second iterator.
func NewRun(values ...core.Primitive) core.Primitive { return transport.NewIO(values...) }

// Drain is the test boundary. Results are observed during delivery, before a
// later update can change a retained reference.
func Drain(t *testing.T, operation, input core.Primitive) []any {
	t.Helper()
	values := []any{}
	core.Yield(transport.NewIO(core.From(0)), transport.NewApply(operation, input),
		func(held int, value core.Primitive) int { values = append(values, core.To[any](value)); return held })
	return values
}
