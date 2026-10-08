package transport_test

import (
	"errors"
	"iter"
	"testing"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/transport"
)

type testBinaryOp struct {
	*core.PrimitiveError
	op func(a, b float64) float64
}

func newTestAdd() core.Primitive {
	return &testBinaryOp{PrimitiveError: core.NewPrimitiveError(), op: func(a, b float64) float64 { return a + b }}
}

func newTestSub() core.Primitive {
	return &testBinaryOp{PrimitiveError: core.NewPrimitiveError(), op: func(a, b float64) float64 { return a - b }}
}

func (op *testBinaryOp) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		var vals []float64
		for p := range in {
			if p != nil {
				vals = append(vals, *(*float64)(p))
			}
		}
		if len(vals) < 2 {
			op.Error(core.ErrShape)
			return
		}
		res := op.op(vals[0], vals[1])
		yield(unsafe.Pointer(&res))
	}
}

func TestMap(t *testing.T) {
	op := transport.NewMap(2, newTestSub())
	want := []float64{5, 6}
	index := 0
	for pointer := range op.Next(data.NewValue(7.0, 2.0, 9.0, 3.0).Next(nil)) {
		if index >= len(want) || *(*float64)(pointer) != want[index] {
			t.Fatal("wrong map")
		}
		index++
	}
	if index != 2 || op.Error() != nil {
		t.Fatalf("count=%d error=%v", index, op.Error())
	}
}
func TestMapPrefix(t *testing.T) {
	op := transport.NewMap(1, newTestSub(), 1)
	index := 0
	want := []float64{8, 7}
	for pointer := range op.Next(data.NewValue(10.0, 2.0, 3.0).Next(nil)) {
		if index >= len(want) || *(*float64)(pointer) != want[index] {
			t.Fatal("wrong prefix")
		}
		index++
	}
	if index != 2 {
		t.Fatal(index)
	}
}
func TestMapRejectsTruncatedRecord(t *testing.T) {
	op := transport.NewMap(2, newTestAdd())
	count := 0
	for range op.Next(data.NewValue(1.0, 2.0, 3.0).Next(nil)) {
		count++
	}
	if count != 0 || !errors.Is(op.Error(), core.ErrShape) {
		t.Fatalf("count=%d error=%v", count, op.Error())
	}
}
