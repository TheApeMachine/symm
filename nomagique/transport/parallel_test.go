package transport_test

import (
	"errors"
	"testing"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestParallel(t *testing.T) {
	op := transport.NewParallel(newTestAdd(), newTestSub())
	want := []float64{3, 5}
	index := 0
	for pointer := range op.Next(data.NewValue[core.Primitive](data.NewValue(1.0, 2.0), data.NewValue(9.0, 4.0)).Next(nil)) {
		if index >= len(want) || *(*float64)(pointer) != want[index] {
			t.Fatal("incorrect route")
		}
		index++
	}
	if index != 2 || op.Error() != nil {
		t.Fatalf("count=%d error=%v", index, op.Error())
	}
}
func TestParallelRejectsBranchArity(t *testing.T) {
	for _, count := range []int{0, 1, 3} {
		op := transport.NewParallel(newTestAdd(), newTestAdd())
		groups := make([]core.Primitive, count)
		for index := range groups {
			groups[index] = data.NewValue(1.0, 2.0)
		}
		emitted := 0
		for range op.Next(data.NewValue(groups...).Next(nil)) {
			emitted++
		}
		if emitted != 0 || !errors.Is(op.Error(), core.ErrShape) {
			t.Fatalf("count=%d output=%d error=%v", count, emitted, op.Error())
		}
	}
}
