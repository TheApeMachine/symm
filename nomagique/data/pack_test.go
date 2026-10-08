package data_test

import (
	"errors"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"testing"
	"unsafe"
)

func TestPackCopiesMutableArrivals(t *testing.T) {
	pack := data.NewPack[float64]()
	var group core.Primitive
	source := func(yield func(unsafe.Pointer) bool) {
		value := 0.0
		for index := 1; index <= 3; index++ {
			value = float64(index)
			if !yield(unsafe.Pointer(&value)) {
				return
			}
		}
	}
	for pointer := range pack.Next(source) {
		group = *(*core.Primitive)(pointer)
	}
	for replay := 0; replay < 2; replay++ {
		index := 1
		for pointer := range group.Next(nil) {
			if *(*float64)(pointer) != float64(index) {
				t.Fatal("pointer aliasing")
			}
			index++
		}
		if index != 4 {
			t.Fatal("incomplete replay")
		}
	}
}
func TestPackRejectsNilWithoutPartialOutput(t *testing.T) {
	op := data.NewPack[float64]()
	count := 0
	for range op.Next(func(yield func(unsafe.Pointer) bool) {
		value := 1.0
		if !yield(unsafe.Pointer(&value)) {
			return
		}
		yield(nil)
	}) {
		count++
	}
	if count != 0 || !errors.Is(op.Error(), core.ErrShape) {
		t.Fatalf("count=%d error=%v", count, op.Error())
	}
}

func TestPackRequiredArity(t *testing.T) {
	for _, count := range []int{0, 1, 2, 3} {
		op := data.NewPack[float64](2)
		input := make([]float64, count)
		groups := 0
		for range op.Next(data.NewValue(input...).Next(nil)) {
			groups++
		}
		if count < 2 && (groups != 0 || op.Error() != nil) {
			t.Fatalf("incomplete group: %d %v", groups, op.Error())
		}
		if count == 2 && (groups != 1 || op.Error() != nil) {
			t.Fatalf("complete group: %d %v", groups, op.Error())
		}
		if count > 2 && (groups != 0 || !errors.Is(op.Error(), core.ErrShape)) {
			t.Fatalf("excess group: %d %v", groups, op.Error())
		}
	}
}
