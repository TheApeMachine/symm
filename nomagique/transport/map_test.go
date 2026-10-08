package transport_test

import (
	"errors"
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/transport"
	"testing"
)

func TestMap(t *testing.T) {
	op := transport.NewMap(2, arithmetic.NewSubtract())
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
	op := transport.NewMap(1, arithmetic.NewSubtract(), 1)
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
	op := transport.NewMap(2, arithmetic.NewAdd())
	count := 0
	for range op.Next(data.NewValue(1.0, 2.0, 3.0).Next(nil)) {
		count++
	}
	if count != 0 || !errors.Is(op.Error(), core.ErrShape) {
		t.Fatalf("count=%d error=%v", count, op.Error())
	}
}
