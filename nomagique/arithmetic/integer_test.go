package arithmetic

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
	"testing"
)

func TestSubtractNanosecondCoordinates(t *testing.T) {
	const at int64 = 1700000000000000007
	result := tests.Drain(t, NewSubtract[int64](transport.NewIO(core.From(at))), transport.NewIO(core.From(at-7)))
	if result[0].(int64) != 7 {
		t.Fatal(result)
	}
}
