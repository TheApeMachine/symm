package transport_test

import (
	"errors"
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
	"testing"
)

func TestFanOneShotSource(t *testing.T) {
	visits := 0
	source := transport.NewGenerator(func(yield func(float64) bool) {
		for _, v := range []float64{1, 2, 3} {
			visits++
			if !yield(v) {
				return
			}
		}
	})
	fan := transport.NewFan(transport.NewPipe(), transport.NewIO(
		arithmetic.NewAdd[float64](transport.NewIO(core.From(0.0))),
		transport.NewMapReduce(calculus.NewSquare(transport.NewIO(core.From(0.0))), arithmetic.NewAdd[float64](transport.NewIO(core.From(0.0)))),
	))
	out := tests.Drain(t, fan, source)
	if visits != 3 {
		t.Fatalf("source visited %d times", visits)
	}
	if len(out) != 2 {
		t.Fatalf("outputs=%v", out)
	}
	tests.EqualNumber(t, out[0], 6)
	tests.EqualNumber(t, out[1], 14)
	if source.Next(nil) != nil {
		t.Fatal("one-shot source replayed")
	}
}
func TestFanIn(t *testing.T) {
	in := transport.NewIO(core.From(1.0), core.From(2.0), core.From(3.0))
	fan := transport.NewFan(transport.NewPipe(), transport.NewIO(arithmetic.NewAdd[float64](transport.NewIO(core.From(0.0)))))
	out := tests.Drain(t, fan, in)
	if len(out) != 1 {
		t.Fatal(out)
	}
	tests.EqualNumber(t, out[0], 6)
}
func TestIOBatchBoundary(t *testing.T) {
	a, b := core.From(1.0), core.From(2.0)
	stream := transport.NewIO(a, nil, b)
	if stream.Next(nil) != a || stream.Next(nil) != nil {
		t.Fatal("first batch")
	}
	if stream.Next(nil) != b || stream.Next(nil) != nil {
		t.Fatal("next batch")
	}
	if stream.Next(nil) != a {
		t.Fatal("new cycle")
	}
}
func TestMapMultipleYields(t *testing.T) {
	mapper := transport.NewMap(transport.NewFan(transport.NewPipe(), transport.NewIO(transport.NewPipe(), transport.NewPipe())))
	out := tests.Drain(t, mapper, transport.NewIO(core.From(2.0), core.From(5.0)))
	if len(out) != 4 {
		t.Fatal(out)
	}
	for i, want := range []float64{2, 2, 5, 5} {
		tests.EqualNumber(t, out[i], want)
	}
}
func TestTerminalErrorSurvives(t *testing.T) {
	marker := errors.New("terminal source error")
	var source *transport.Generator[float64]
	source = transport.NewGenerator(func(yield func(float64) bool) { yield(2); source.Error(marker) })
	operation := arithmetic.NewAdd[float64](transport.NewIO(core.From(0.0)))
	result := operation.Next(source)
	if !errors.Is(result.Error(), marker) || !errors.Is(operation.Error(), marker) {
		t.Fatal("lost terminal error")
	}
	if operation.Next(source) != nil {
		t.Fatal("delivery run did not end")
	}
}
func TestZipRejectsUnequalBranches(t *testing.T) {
	zip := transport.NewZip(transport.NewPipe(), transport.NewIO(core.From(1.0)))
	result := zip.Next(transport.NewIO(core.From(1.0), core.From(2.0)))
	if result != nil || !errors.Is(zip.Error(), core.ErrShape) {
		t.Fatal("unequal zip accepted")
	}
}

func TestTransportNeverDecodesOpaqueObjects(t *testing.T) {
	object := tests.NewOpaque()
	fan := transport.NewFan(transport.NewPipe(), transport.NewIO(transport.NewPipe(), transport.NewPipe()))
	seen := 0
	out := core.Yield(transport.NewIO(core.From(0)), transport.NewApply(fan, transport.NewIO(object)), func(n int, value core.Primitive) int {
		if value != object {
			t.Fatal("object identity changed")
		}
		seen++
		return n
	})
	if out.Error() != nil {
		t.Fatal(out.Error())
	}
	if seen != 2 {
		t.Fatalf("deliveries=%d", seen)
	}
}
