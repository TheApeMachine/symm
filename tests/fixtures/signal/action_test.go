package signal

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

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

	Convey("Given an invalid multi-action draft", t, func() {
		signal := New([]string{"SIM1/USD"})
		signal.Bootstrap()
		before := []Sample{}

		for samples := range signal.Generate() {
			before = samples
		}

		at := signal.Now()
		err := signal.Apply(Step{
			Advance: time.Second,
			Actions: []Action{
				{Kind: Refill, Symbol: "SIM1/USD", Side: "buy", Qty: 5},
				{Kind: Refill, Symbol: "UNKNOWN/USD", Side: "buy", Qty: 5},
			},
		})

		Convey("It should leave the complete venue unchanged", func() {
			So(err, ShouldNotBeNil)
			So(signal.Now(), ShouldEqual, at)

			for samples := range signal.Generate() {
				So(samples, ShouldResemble, before)
			}
		})

		Convey("It should reject empty sides without panicking", func() {
			cancels := []Action{}

			for _, order := range before[0].Asks {
				cancels = append(cancels, Action{
					Kind: Cancel, Symbol: "SIM1/USD", OrderID: order.ID,
				})
			}

			So(signal.Apply(Step{Advance: time.Second, Actions: cancels}), ShouldNotBeNil)
			So(signal.Now(), ShouldEqual, at)
		})

		Convey("It should consume a complete touch and continue through the next level", func() {
			quantity := before[0].Asks[0].Qty + 5
			So(signal.Apply(Step{
				Advance: time.Second,
				Actions: []Action{{
					Kind: Trade, Symbol: "SIM1/USD", Side: "buy", Qty: quantity,
				}},
			}), ShouldBeNil)

			for samples := range signal.Generate() {
				So(samples[0].Fills, ShouldHaveLength, 2)
				So(samples[0].Fills[0].Price, ShouldEqual, before[0].Asks[0].Price)
				So(samples[0].Fills[0].Qty, ShouldEqual, before[0].Asks[0].Qty)
				So(samples[0].Fills[1].Price, ShouldEqual, before[0].Asks[1].Price)
				So(samples[0].Fills[1].Qty, ShouldEqual, 5)
				So(samples[0].Asks, ShouldHaveLength, 1)
				So(samples[0].Asks[0].Qty, ShouldEqual, before[0].Asks[1].Qty-5)
			}
		})

		Convey("It should reject quantities outside the venue grid", func() {
			So(signal.Apply(Step{
				Advance: time.Second,
				Actions: []Action{{
					Kind: Trade, Symbol: "SIM1/USD", Side: "buy",
					Qty: QuantityIncrement / 2,
				}},
			}), ShouldNotBeNil)
			So(signal.Now(), ShouldEqual, at)
		})

		Convey("It should reject prices outside the venue tick lattice", func() {
			So(signal.Apply(Step{
				Advance: time.Second,
				Actions: []Action{{
					Kind: Add, Symbol: "SIM1/USD", Side: "buy",
					Price: initialPrice + PriceIncrement/2, Qty: 1,
				}},
			}), ShouldNotBeNil)
			So(signal.Now(), ShouldEqual, at)
		})
	})
}

/*
BenchmarkSignal_Apply measures complete-touch removal and second-level filling.
*/
func BenchmarkSignal_Apply(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		signal := New([]string{"SIM1/USD"})
		signal.Bootstrap()
		quote, _ := signal.Quote("SIM1/USD")

		if err := signal.Apply(Step{
			Advance: time.Second,
			Actions: []Action{{
				Kind:   Trade,
				Symbol: "SIM1/USD",
				Side:   "buy",
				Qty:    quote.AskQty + 5,
			}},
		}); err != nil {
			b.Fatal(err)
		}
	}
}
