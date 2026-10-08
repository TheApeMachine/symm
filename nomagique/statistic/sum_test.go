package statistic

import (
	"errors"
	"iter"
	"reflect"
	"testing"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestSumNextRetainsTotalAcrossRuns(t *testing.T) {
	op := NewSum()
	got := make([]float64, 0)
	for _, delta := range []float64{10, -3, 5, -12} {
		for ptr := range op.Next(data.NewValue(delta).Next(nil)) {
			got = append(got, *(*float64)(ptr))
		}
	}
	want := []float64{10, 7, 12, 0}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("running sums = %v, want %v", got, want)
	}
}

func TestSumNextEmptyRunHasNoObservation(t *testing.T) {
	op := NewSum()
	for range op.Next(data.NewValue(2.0).Next(nil)) {
	}
	for range op.Next(data.NewValue[float64]().Next(nil)) {
		t.Fatal("empty input fabricated an observation")
	}
	got := data.Read[float64](op.Next(data.NewValue(3.0).Next(nil)))
	if got != 5 {
		t.Fatalf("sum after empty run = %g, want 5", got)
	}
}

func TestSumNextShapeErrorDoesNotCommitPartialRun(t *testing.T) {
	op := NewSum().(*Sum)
	for range op.Next(data.NewValue(2.0).Next(nil)) {
	}
	bad := iter.Seq[unsafe.Pointer](func(yield func(unsafe.Pointer) bool) {
		value := 100.0
		if !yield(unsafe.Pointer(&value)) {
			return
		}
		yield(nil)
	})
	for range op.Next(bad) {
		t.Fatal("invalid run yielded a number")
	}
	if op.total != 2 {
		t.Fatalf("malformed run committed partial sum: %g, want 2", op.total)
	}
	if !errors.Is(op.Error(), core.ErrShape) {
		t.Fatalf("error = %v", op.Error())
	}
}
