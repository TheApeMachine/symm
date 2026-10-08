package distribution_test

import (
	"errors"
	"math"
	"math/rand"
	"sort"
	"testing"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/distribution"
)

func collect(op core.Primitive, values ...float64) []float64 {
	var out []float64
	for pointer := range op.Next(data.NewValue(values...).Next(nil)) {
		out = append(out, *(*float64)(pointer))
	}
	return out
}
func compare(op core.Primitive, left, right []float64) []float64 {
	var out []float64
	for pointer := range op.Next(data.NewValue[core.Primitive](data.NewValue(left...), data.NewValue(right...)).Next(nil)) {
		out = append(out, *(*float64)(pointer))
	}
	return out
}
func equal(t *testing.T, got, want []float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for index := range want {
		if math.IsNaN(got[index]) || math.Abs(got[index]-want[index]) > 1e-10 {
			t.Fatalf("at %d: got %.17g, want %.17g", index, got[index], want[index])
		}
	}
}
func TestNormalize(t *testing.T) {
	equal(t, collect(distribution.NewNormalize(), 1, 2, 1), []float64{.25, .5, .25})
	equal(t, collect(distribution.NewNormalize(2, 1), -2, 1, 3, 3), []float64{-2, .25, 3, .75})
	for _, values := range [][]float64{nil, {0, 0}} {
		op := distribution.NewNormalize()
		equal(t, collect(op, values...), nil)
		if op.Error() != nil {
			t.Fatal(op.Error())
		}
	}
	op := distribution.NewNormalize()
	equal(t, collect(op, 1, -1), nil)
	if !errors.Is(op.Error(), core.ErrDomain) {
		t.Fatal(op.Error())
	}
	op = distribution.NewNormalize(2, 1)
	equal(t, collect(op, 0, 1, 2), nil)
	if !errors.Is(op.Error(), core.ErrShape) {
		t.Fatal(op.Error())
	}
}
func TestSortedPositions(t *testing.T) {
	equal(t, collect(distribution.NewSortedPositions(), 3, 4, -1, 2, 3, 5), []float64{-1, 2, 3, 4, 3, 5})
	op := distribution.NewSortedPositions()
	equal(t, collect(op, 0), nil)
	if !errors.Is(op.Error(), core.ErrShape) {
		t.Fatal(op.Error())
	}
}
func TestMergedWalk(t *testing.T) {
	equal(t, compare(distribution.NewMergedWalk(), []float64{0, 1}, []float64{2, 1}), []float64{1, 2, 2})
	equal(t, compare(distribution.NewMergedWalk(), []float64{0, 1, 0, 1, 2, 2}, []float64{0, 1, 2, 1}), []float64{0, 0, 2})
	equal(t, compare(distribution.NewMergedWalk(), []float64{-3, 1, 0, 1}, []float64{-3, 3, 1, 1}), []float64{.25, 1, 3})
	for _, sides := range [][2][]float64{{nil, {0, 1}}, {{0, 0}, {0, 1}}} {
		op := distribution.NewMergedWalk()
		equal(t, compare(op, sides[0], sides[1]), nil)
		if op.Error() != nil {
			t.Fatal(op.Error())
		}
	}
	for _, bad := range [][]float64{{2, 1, 1, 1}, {0, -1}, {0}} {
		op := distribution.NewMergedWalk()
		equal(t, compare(op, bad, []float64{0, 1}), nil)
		if op.Error() == nil {
			t.Fatalf("accepted %v", bad)
		}
	}
}
func TestDistanceSelectors(t *testing.T) {
	left, right := []float64{0, 1, 2, 1}, []float64{1, 1, 3, 1}
	equal(t, compare(distribution.NewWasserstein1Pairs(), left, right), []float64{1})
	equal(t, compare(distribution.NewKolmogorovSmirnovPairs(), left, right), []float64{.5})
	equal(t, collect(distribution.NewWasserstein1(), 0, 1, 0, 1, 0, 1, 2, 1, 0, 3, 0, 1), []float64{1})
	equal(t, collect(distribution.NewKolmogorovSmirnov(), 0, 1, 0, 1, 0, 1, 2, 1, 0, 3, 0, 1), []float64{.5})
}
func TestEntropyAndConcentration(t *testing.T) {
	equal(t, collect(distribution.NewEntropy(), .25, .25, .25, .25), []float64{math.Log(4)})
	equal(t, collect(distribution.NewConcentration(), .25, .25, .25, .25), []float64{.25})
	equal(t, collect(distribution.NewEntropyPoints(), 0, 2, 5, 2), []float64{math.Log(2)})
	equal(t, collect(distribution.NewConcentrationPoints(), 0, 2, 5, 2), []float64{.5})
	equal(t, collect(distribution.NewEntropyPoints(), 0, 0), nil)
	equal(t, collect(distribution.NewConcentrationPoints(), 0, 0), nil)
	equal(t, collect(distribution.NewEntropy(), 1), []float64{0})
}

// Independent reference: explicit union CDF, deliberately unlike the production
// two-stream merged walk. Only test code materializes the union support.
func reference(left, right []float64) (float64, float64) {
	mass := map[float64][2]float64{}
	totals := [2]float64{}
	for side, values := range [2][]float64{left, right} {
		for index := 0; index < len(values); index += 2 {
			point := mass[values[index]]
			point[side] += values[index+1]
			mass[values[index]] = point
			totals[side] += values[index+1]
		}
	}
	support := make([]float64, 0, len(mass))
	for position := range mass {
		support = append(support, position)
	}
	sort.Float64s(support)
	cumulative := [2]float64{}
	distance, ks := 0.0, 0.0
	for index, position := range support {
		point := mass[position]
		cumulative[0] += point[0] / totals[0]
		cumulative[1] += point[1] / totals[1]
		gap := math.Abs(cumulative[0] - cumulative[1])
		ks = math.Max(ks, gap)
		if index+1 < len(support) {
			distance += gap * (support[index+1] - position)
		}
	}
	return ks, distance
}
func TestMergedWalkAgainstIndependentCDF(t *testing.T) {
	random := rand.New(rand.NewSource(4189))
	for trial := 0; trial < 1000; trial++ {
		var sides [2][]float64
		for side := range sides {
			count := 1 + random.Intn(30)
			positions := make([]float64, count)
			for index := range positions {
				positions[index] = float64(random.Intn(21) - 10)
			}
			sort.Float64s(positions)
			for _, position := range positions {
				sides[side] = append(sides[side], position, float64(1+random.Intn(10)))
			}
		}
		ks, distance := reference(sides[0], sides[1])
		got := compare(distribution.NewMergedWalk(), sides[0], sides[1])
		equal(t, got[:2], []float64{ks, distance})
		reverse := compare(distribution.NewMergedWalk(), sides[1], sides[0])
		equal(t, reverse[:2], got[:2])
	}
}
func TestMergedWalkCancellation(t *testing.T) {
	op := distribution.NewMergedWalk()
	calls := 0
	run := op.Next(data.NewValue[core.Primitive](data.NewValue(0.0, 1.0), data.NewValue(1.0, 1.0)).Next(nil))
	run(func(pointer unsafe.Pointer) bool { calls++; return false })
	if calls != 1 {
		t.Fatalf("yielded after stop: %d", calls)
	}
}
