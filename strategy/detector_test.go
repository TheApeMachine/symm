package strategy

import (
	"context"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
)

/*
dogePrice is an offline price system for DOGE/USD with a 0.1% taker fee and
$1000 of reference cash.
*/
func dogePrice() *broker.Price {
	normalizer := spot.NewNormalizer()
	tickSize, _ := decimal.NewFromString("0.00001")
	normalizer.Update(&spot.AssetsManagerUpdate{
		NewAssets: map[string]spot.AssetInfo{"XDG": {AltName: "DOGE"}, "USD": {AltName: "USD"}},
		NewPairs: map[string]spot.AssetPair{"XDGUSD": {
			WSName: "DOGE/USD", Base: "XDG", Quote: "USD",
			PairDecimals: 5, LotDecimals: 8, LotMultiplier: 1, TickSize: tickSize,
		}},
	})

	price := broker.NewPrice(context.Background(), nil, nil, nil, normalizer)
	price.SetFee("DOGE/USD", kraken.TradeVolumeFee{Fee: decimal.NewFromFloat64(0.1)})
	price.SetReferenceCash(decimal.NewFromFloat64(1000))

	return price
}

/*
detectTape feeds DOGE/USD one trade per tick from symbolFirstTick on, at the
given prices, with a touch around each, and returns what the detector
emitted (including on Close).
*/
func detectTape(symbolFirstTick uint64, prices []float64) ([]*data.Value, *Detector) {
	detector, err := NewDetector(context.Background(), dogePrice(), 7, "DOGE/USD", system.Cfg.Detector)
	So(err, ShouldBeNil)

	out := make([]*data.Value, 0)

	for idx, p := range prices {
		tick := symbolFirstTick + uint64(idx)
		results, err := detector.Step(tick, kraken.TradeData{
			Symbol: "DOGE/USD", Side: "buy", Price: *decimal.NewFromFloat64(p), Qty: 1,
			OrderType: "market", TradeID: int64(idx + 1), Timestamp: time.Unix(int64(tick), 0).UTC(),
		}, broker.PriceLayer{Symbol: "DOGE/USD", Bid: decimal.NewFromFloat64(p * 0.9999), Ask: decimal.NewFromFloat64(p * 1.0001)})
		So(err, ShouldBeNil)
		out = append(out, results...)
	}

	results, err := detector.Close()
	So(err, ShouldBeNil)

	return append(out, results...), detector
}

/*
igniteEarly is a tape whose up leg starts 15 trades after the symbol's
first trade and runs 400 ticks: half the move (200 ticks) reaches far back
before the symbol ever traded.
*/
func igniteEarly() []float64 {
	prices := make([]float64, 0, 900)

	for idx := range 15 {
		prices = append(prices, 101-float64(idx)/14)
	}

	for idx := range 400 {
		prices = append(prices, 100+40*float64(idx+1)/400)
	}

	for idx := range 50 {
		prices = append(prices, 140-20*float64(idx+1)/50)
	}

	for range 400 {
		prices = append(prices, 120)
	}

	return prices
}

func TestDetectorPrecursor(t *testing.T) {
	// Daniel's detect run on epoch 1792189318815706000 dropped liquid
	// symbols' excursions with "no <SYM> trade at or before entry tick": A
	// was clamped to the epoch's first tick, so a leg igniting soon after the
	// symbol's first trade got a precursor (and a lead) reaching back before
	// the symbol ever traded.
	Convey("Given a symbol whose first trade comes long after its epoch's first tick", t, func() {
		So(types.Focus(), ShouldNotBeNil)

		Convey("An up leg igniting soon after its first trade is emitted, its precursor clamped to that trade", func() {
			results, detector := detectTape(1000, igniteEarly())

			var up *data.Value

			for _, result := range results {
				fields, err := data.ExcursionFields(result)
				So(err, ShouldBeNil)

				// Nothing reaches back before the symbol's first trade.
				So(fields["a_tick"], ShouldBeGreaterThanOrEqualTo, 1000)
				So(fields["entry_tick"], ShouldBeGreaterThanOrEqualTo, fields["a_tick"])
				So(fields["entry_tick"], ShouldBeLessThanOrEqualTo, fields["b_tick"])

				if result.Role == "up" {
					up = result
				}
			}

			So(up, ShouldNotBeNil)
			fields, _ := data.ExcursionFields(up)
			So(fields["a_tick"], ShouldEqual, 1000)
			So(fields["a_idx"], ShouldEqual, 1)
			So(detector.Insufficient(), ShouldBeLessThanOrEqualTo, 1)
		})

		Convey("The excursions depend on the symbol's own tape, not where it starts in the epoch", func() {
			late, _ := detectTape(1000, igniteEarly())
			early, _ := detectTape(1, igniteEarly())
			So(len(early), ShouldEqual, len(late))

			for idx := range late {
				lateFields, err := data.ExcursionFields(late[idx])
				So(err, ShouldBeNil)
				earlyFields, err := data.ExcursionFields(early[idx])
				So(err, ShouldBeNil)
				So(early[idx].Role, ShouldEqual, late[idx].Role)
				So(earlyFields["entry_tick"]+999, ShouldEqual, lateFields["entry_tick"])
				So(earlyFields["a_tick"]+999, ShouldEqual, lateFields["a_tick"])
			}
		})
	})
}
