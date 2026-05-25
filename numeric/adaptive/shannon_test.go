package adaptive

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestShannonEntropy(t *testing.T) {
	shannon := NewShannon()

	t.Parallel()

	Convey("Given ShannonEntropy on nonnegative counts", t, func() {
		Convey("It should return zero for an all-zero vector", func() {
			var vec [8]uint64

			So(shannon.Entropy(vec), ShouldEqual, 0)
		})

		Convey("It should reach log2(K) for K equal bins", func() {
			var vec [8]uint64

			vec[1] = 5
			vec[2] = 5
			vec[3] = 5
			vec[4] = 5

			So(shannon.Entropy(vec), ShouldAlmostEqual, 2.0, 1e-12)
		})
	})
}

func TestShannonEntropyBitsFromMap(t *testing.T) {
	shannon := NewShannon()

	t.Parallel()

	Convey("Given ShannonEntropyBitsFromMap", t, func() {
		Convey("It should normalize map scores by probabilityScale", func() {
			scores := map[string]float64{
				"a": 50,
				"b": 50,
			}

			h := shannon.EntropyBitsFromMap(scores, 1.0/100.0)

			So(h, ShouldAlmostEqual, 1.0, 1e-12)
		})

		Convey("It should return NaN when scale is non-positive", func() {
			h := shannon.EntropyBitsFromMap(map[string]float64{"x": 1}, 0)

			So(math.IsNaN(h), ShouldBeTrue)
		})

		Convey("It should return NaN when any entry is negative", func() {
			h := shannon.EntropyBitsFromMap(map[string]float64{"x": -1}, 0.5)

			So(math.IsNaN(h), ShouldBeTrue)
		})
	})
}

func BenchmarkShannonEntropy(b *testing.B) {
	shannon := NewShannon()

	vec := [8]uint64{1, 2, 3, 4, 5, 6, 7, 8}

	var h float64

	for range b.N {
		h = shannon.Entropy(vec)
	}

	_ = h
}

func BenchmarkShannonEntropyBitsFromMap(b *testing.B) {
	shannon := NewShannon()

	scores := map[string]float64{
		"a": 25, "b": 25, "c": 25, "d": 25,
	}

	var h float64

	for range b.N {
		h = shannon.EntropyBitsFromMap(scores, 1.0/100.0)
	}

	_ = h
}
