package signal

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

/*
TestSignal_Transition proves semantic states produce deterministic price paths.
*/
func TestSignal_Transition(t *testing.T) {
	Convey("Given identical deterministic market signals", t, func() {
		left := New([]string{"SIM1/USD"})
		right := New([]string{"SIM1/USD"})
		leftPrices := []float64{}
		rightPrices := []float64{}
		left.Transition(FastPump)
		right.Transition(FastPump)

		for samples := range left.Generate() {
			leftPrices = append(leftPrices, samples[0].Price)
		}

		for samples := range right.Generate() {
			rightPrices = append(rightPrices, samples[0].Price)
		}

		Convey("When both transition into a fast pump", func() {
			So(leftPrices, ShouldResemble, rightPrices)
			So(leftPrices, ShouldHaveLength, fastLegObservations+settleObservations)
			So(leftPrices[len(leftPrices)-1], ShouldBeGreaterThan, leftPrices[0])
			So(leftPrices[fastLegObservations-1], ShouldAlmostEqual, initialPrice*(1+eventMoveFraction), 0.1)
			So(leftPrices[len(leftPrices)-1], ShouldAlmostEqual, initialPrice*(1+eventMoveFraction), 0.1)
		})

		Convey("When one transitions into a fast dump", func() {
			prices := []float64{}
			left.Transition(FastDump)

			for samples := range left.Generate() {
				prices = append(prices, samples[0].Price)
			}

			So(prices[len(prices)-1], ShouldBeLessThan, prices[0])
		})
	})

	Convey("Given an idle market with several symbols", t, func() {
		signal := New([]string{"SIM1/USD", "SIM2/USD", "SIM3/USD"})
		signal.Transition(Baseline)
		prices := map[string][]float64{}

		for samples := range signal.Generate() {
			for _, sample := range samples {
				prices[sample.Symbol] = append(prices[sample.Symbol], sample.Price)
			}
		}

		Convey("It should gently oscillate every symbol without changing its level", func() {
			So(prices, ShouldHaveLength, 3)

			for _, path := range prices {
				So(path, ShouldHaveLength, idleObservations)
				So(path[0], ShouldNotEqual, path[1])
				So(path[0], ShouldAlmostEqual, initialPrice, initialPrice*idleAmplitudeFraction*2)
				So(path[len(path)-1], ShouldAlmostEqual, initialPrice, initialPrice*idleAmplitudeFraction*2)
			}
		})
	})
}

/*
BenchmarkSignal_Transition measures the fixture price and clock generator.
*/
func BenchmarkSignal_Transition(b *testing.B) {
	signal := New([]string{"SIM1/USD", "SIM2/USD", "SIM3/USD"})
	b.ReportAllocs()

	for b.Loop() {
		signal.Transition(Baseline)

		for samples := range signal.Generate() {
			if samples[0].At.IsZero() {
				b.Fatal("transition timestamp is zero")
			}
		}
	}
}
