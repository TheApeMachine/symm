package resonance

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRingCapacity(testingTB *testing.T) {
	cases := []struct {
		name     string
		samples  []float64
		expected int
	}{
		{name: "empty", samples: nil, expected: 1},
		{name: "single", samples: []float64{1}, expected: 2},
		{name: "pair", samples: []float64{1, 2}, expected: 3},
		{name: "wide span grows capacity", samples: []float64{1, 2, 3, 4, 5}, expected: 5},
	}

	for _, testCase := range cases {
		Convey("Given ring capacity inputs "+testCase.name, testingTB, func() {
			So(ringCapacity(testCase.samples), ShouldEqual, testCase.expected)
		})
	}
}

func TestRatioToMedian(testingTB *testing.T) {
	cases := []struct {
		name     string
		values   []float64
		next     float64
		expected float64
	}{
		{name: "non-finite value", values: nil, next: math.NaN(), expected: 0},
		{name: "non-positive value", values: nil, next: 0, expected: 0},
		{name: "first observation defaults to one", values: nil, next: 10, expected: 1},
		{name: "tracks median ratio", values: []float64{10, 10, 10}, next: 20, expected: 2},
	}

	for _, testCase := range cases {
		Convey("Given ratio-to-median case "+testCase.name, testingTB, func() {
			ring := scalarRing{samples: append([]float64(nil), testCase.values...)}
			ratio := ratioToMedian(testCase.next, &ring)

			So(ratio, ShouldAlmostEqual, testCase.expected, 1e-9)
		})
	}
}

func TestScaledSigned(testingTB *testing.T) {
	cases := []struct {
		name     string
		values   []float64
		next     float64
		expected float64
	}{
		{name: "non-finite", values: nil, next: math.Inf(1), expected: 0},
		{name: "first sample self-normalizes to one", values: nil, next: 4, expected: 1},
		{name: "scales by median absolute", values: []float64{2, -2, 2, -2}, next: 4, expected: 2},
	}

	for _, testCase := range cases {
		Convey("Given scaled-signed case "+testCase.name, testingTB, func() {
			ring := scalarRing{samples: append([]float64(nil), testCase.values...)}
			scaled := scaledSigned(testCase.next, &ring)

			So(scaled, ShouldAlmostEqual, testCase.expected, 1e-9)
		})
	}
}

func TestScalarRingTrimming(testingTB *testing.T) {
	Convey("Given repeated observations beyond dynamic capacity", testingTB, func() {
		ring := scalarRing{}

		for index := range 20 {
			ring.observe(float64(index + 1))
		}

		capacity := ringCapacity(ring.samples)

		Convey("It should retain only the trailing window", func() {
			So(len(ring.samples), ShouldBeLessThanOrEqualTo, capacity)
			So(ring.samples[len(ring.samples)-1], ShouldEqual, 20)
		})
	})
}

func TestSenseRegistryBaselines(testingTB *testing.T) {
	Convey("Given a sense registry", testingTB, func() {
		registry := newSenseRegistry()

		first := registry.baselines("BTC/USD")
		second := registry.baselines("BTC/USD")
		other := registry.baselines("ETH/USD")

		first.changeAbs.observe(1.5)

		Convey("It should reuse baselines per symbol and isolate symbols", func() {
			So(first, ShouldEqual, second)
			So(len(first.changeAbs.samples), ShouldEqual, 1)
			So(len(other.changeAbs.samples), ShouldEqual, 0)
		})
	})
}

func BenchmarkRatioToMedian(b *testing.B) {
	ring := scalarRing{}

	for index := range 64 {
		ring.observe(float64(index + 1))
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = ratioToMedian(float64(b.N%10+1), &ring)
	}
}
