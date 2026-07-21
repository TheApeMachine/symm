package signal

import (
	"testing"
	"time"

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
		So(left.Transition(FastPump), ShouldBeNil)
		So(right.Transition(FastPump), ShouldBeNil)

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
			So(left.Transition(FastDump), ShouldBeNil)

			for samples := range left.Generate() {
				prices = append(prices, samples[0].Price)
			}

			So(prices[len(prices)-1], ShouldBeLessThan, prices[0])
		})
	})

	Convey("Given an idle market with several symbols", t, func() {
		signal := New([]string{"SIM1/USD", "SIM2/USD", "SIM3/USD"})
		So(signal.Transition(Baseline), ShouldBeNil)
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
TestSignal_Apply proves explicit economic actions mutate the single book and
trade state consumed by every dynamic fixture.
*/
func TestSignal_Apply(t *testing.T) {
	Convey("Given one bootstrapped authoritative market", t, func() {
		signal := New([]string{"SIM1/USD"})
		signal.Bootstrap()
		var initial Sample

		for samples := range signal.Generate() {
			initial = samples[0]
		}

		bidID := initial.Bids[0].ID
		bidQty := initial.Bids[0].Qty
		startVolume := initial.Statistics.Volume

		Convey("Refilling modifies an existing order without fabricating a trade", func() {
			So(signal.Apply(Step{
				Advance: time.Second,
				Actions: []Action{{
					Kind: Refill, Symbol: "SIM1/USD", Side: "buy", Qty: 5,
				}},
			}), ShouldBeNil)

			for samples := range signal.Generate() {
				So(samples[0].Traded, ShouldBeFalse)
				So(samples[0].Bids[0].ID, ShouldEqual, bidID)
				So(samples[0].Bids[0].Qty, ShouldEqual, bidQty+5)
				So(samples[0].Statistics.Volume, ShouldEqual, startVolume)
			}
		})

		Convey("Adding and cancelling affect only the selected resting identity", func() {
			price := initial.Bids[0].Price
			So(signal.Apply(Step{
				Advance: time.Second,
				Actions: []Action{{
					Kind: Add, Symbol: "SIM1/USD", Side: "buy", Price: price, Qty: 5,
				}},
			}), ShouldBeNil)
			var added string

			for samples := range signal.Generate() {
				So(samples[0].Bids, ShouldHaveLength, len(initial.Bids)+1)
				added = samples[0].Bids[len(samples[0].Bids)-1].ID
			}

			So(signal.Apply(Step{
				Advance: time.Second,
				Actions: []Action{{
					Kind: Cancel, Symbol: "SIM1/USD", OrderID: added,
				}},
			}), ShouldBeNil)

			for samples := range signal.Generate() {
				So(samples[0].Bids, ShouldHaveLength, len(initial.Bids))
			}
		})

		Convey("A buy executes at and consumes the authoritative ask touch", func() {
			ask := initial.Asks[0]
			So(signal.Apply(Step{
				Advance: time.Second,
				Actions: []Action{{
					Kind: Trade, Symbol: "SIM1/USD", Side: "buy", Qty: 5,
				}},
			}), ShouldBeNil)

			for samples := range signal.Generate() {
				So(samples[0].Traded, ShouldBeTrue)
				So(samples[0].TradePrice, ShouldEqual, ask.Price)
				So(samples[0].Asks[0].Qty, ShouldEqual, ask.Qty-5)
				So(samples[0].Statistics.Volume, ShouldEqual, startVolume+5)
			}
		})

		Convey("Widening the spread replaces prices with new resting identities", func() {
			So(signal.Apply(Step{
				Advance: time.Second,
				Actions: []Action{{
					Kind: WidenSpread, Symbol: "SIM1/USD", Ticks: 2,
				}},
			}), ShouldBeNil)

			for samples := range signal.Generate() {
				So(samples[0].Bids[0].Price, ShouldBeLessThan, initial.Bids[0].Price)
				So(samples[0].Asks[0].Price, ShouldBeGreaterThan, initial.Asks[0].Price)
				So(samples[0].Bids[0].ID, ShouldNotEqual, initial.Bids[0].ID)
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
		if err := signal.Transition(Baseline); err != nil {
			b.Fatal(err)
		}

		for samples := range signal.Generate() {
			if samples[0].At.IsZero() {
				b.Fatal("transition timestamp is zero")
			}
		}
	}
}
