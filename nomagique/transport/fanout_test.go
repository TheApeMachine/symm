package transport_test

import (
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/transport"
	"testing"
)

func TestFanout(t *testing.T) {
	op := transport.NewFanout[float64](arithmetic.NewAdd(), arithmetic.NewSubtract())
	want := []float64{9, 5}
	index := 0
	for pointer := range op.Next(data.NewValue(7.0, 2.0).Next(nil)) {
		if index >= len(want) || *(*float64)(pointer) != want[index] {
			t.Fatal("wrong result or order")
		}
		index++
	}
	if index != len(want) || op.Error() != nil {
		t.Fatalf("count=%d error=%v", index, op.Error())
	}
}
