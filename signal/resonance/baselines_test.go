package resonance

import (
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/statistic"
)

func TestScalarRingWindowDepth(testingTB *testing.T) {
	Convey("Given stamp observations", testingTB, func() {
		ring := scalarRing{}

		for index := range 20 {
			ring.observe(float64(index+1), float64((index+1)*1_000_000_000))
		}

		_, keep, err := statistic.ResolveWindows(ring.stamps, 0, 0)
		So(err, ShouldBeNil)

		Convey("It should retain only the trailing stamp window", func() {
			So(len(ring.samples), ShouldEqual, keep)
			So(len(ring.stamps), ShouldEqual, keep)
			So(ring.samples[len(ring.samples)-1], ShouldEqual, 20)
		})
	})
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

			for index := range testCase.values {
				ring.stamps = append(ring.stamps, float64((index+1)*1_000_000_000))
			}

			ratio := ratioToMedian(testCase.next, &ring, float64((len(testCase.values)+1)*1_000_000_000))

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

			for index := range testCase.values {
				ring.stamps = append(ring.stamps, float64((index+1)*1_000_000_000))
			}

			scaled := scaledSigned(testCase.next, &ring, float64((len(testCase.values)+1)*1_000_000_000))

			So(scaled, ShouldAlmostEqual, testCase.expected, 1e-9)
		})
	}
}

func TestSenseRegistryBaselines(testingTB *testing.T) {
	Convey("Given a sense registry", testingTB, func() {
		registry := newSenseRegistry()

		first := registry.baselines("BTC/USD")
		second := registry.baselines("BTC/USD")
		other := registry.baselines("ETH/USD")

		first.changeAbs.observe(1.5, 1_000_000_000)

		Convey("It should reuse baselines per symbol and isolate symbols", func() {
			So(first, ShouldEqual, second)
			So(len(first.changeAbs.samples), ShouldEqual, 1)
			So(len(other.changeAbs.samples), ShouldEqual, 0)
		})
	})
}

func TestSpreadRatioToMedian(testingTB *testing.T) {
	Convey("Given the first positive spread observation", testingTB, func() {
		ring := scalarRing{}
		stamp := float64(1_000_000_000)

		ratio := spreadRatioToMedian(12.5, &ring, stamp)

		Convey("It should seed the baseline and return unity", func() {
			So(ratio, ShouldEqual, 1)
			So(len(ring.samples), ShouldEqual, 1)
			So(ring.samples[0], ShouldEqual, 12.5)
		})
	})
}

func TestSpreadWideRatio(testingTB *testing.T) {
	Convey("Given the first spread observation", testingTB, func() {
		ratio := spreadWideRatio(12.5, nil)

		Convey("It should seed the baseline as unity", func() {
			So(ratio, ShouldEqual, 1)
		})
	})
}

func BenchmarkRatioToMedian(b *testing.B) {
	ring := scalarRing{}

	for index := range 64 {
		ring.observe(float64(index+1), float64((index+1)*1_000_000_000))
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = ratioToMedian(float64(b.N%10+1), &ring, float64(b.N*1_000_000_000))
	}
}
