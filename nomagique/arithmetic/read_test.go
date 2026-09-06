package arithmetic

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
	"testing"
)

func TestReadAfterDeliveryEnds(t *testing.T) {
	add := NewAdd[float64](transport.NewIO(core.From(0.0)))
	tests.Drain(t, add, transport.NewIO(core.From(2.0), core.From(3.0)))
	tests.EqualNumber(t, add.Read(), 5)
}
