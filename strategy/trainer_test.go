package strategy

import (
	"cmp"
	"context"
	"slices"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/tests/tablestest"
)

func TestTrainer(t *testing.T) {
	system.NewConfig()
	ctx := context.Background()

	Convey("Trainer", t, func() {
		normalizer := spot.NewNormalizer()
		normalizer.Update(&spot.AssetsManagerUpdate{
			NewAssets: map[string]spot.AssetInfo{
				"BTC": {AltName: "XBT"},
				"USD": {AltName: "USD"},
			},
			NewPairs: map[string]spot.AssetPair{
				"BTC/USD": {WSName: "BTC/USD", Base: "BTC", Quote: "USD", LotDecimals: 8, LotMultiplier: 1},
			},
		})

		price := broker.NewPrice(ctx, nil, nil, nil, normalizer)
		price.SetFee("BTC/USD", kraken.TradeVolumeFee{Fee: decimal.NewFromFloat64(0.26)})
		price.SetReferenceCash(decimal.NewFromFloat64(200))
		catalog := tablestest.New(t)

		Convey("NewTrainer and SetSymbols", func() {
			trainer := NewTrainer(ctx, catalog, price)
			So(trainer, ShouldNotBeNil)
			So(trainer.admits("BTC/USD"), ShouldBeTrue)

			trainer.SetSymbols("ETH/USD")
			So(trainer.admits("BTC/USD"), ShouldBeFalse)
			So(trainer.admits("ETH/USD"), ShouldBeTrue)
		})

		Convey("parseExcursion extracts ordered venue bounds and return", func() {
			trainer := NewTrainer(ctx, catalog, price)
			now := time.Now().UTC()

			meas := data.NewMeasurement(100, "BTC/USD", "hindsight:excursions", 1, 1)
			meas.From = now
			meas.At = now.Add(10 * time.Second)
			meas.SetMeta("type", "up")
			bPriceMetric := data.NewMetric("b_price", 50000, data.UnitCurrency, data.TimescaleTick)
			bPriceMetric.Exact = decimal.NewFromInt64(50000)
			cPriceMetric := data.NewMetric("c_price", 55000, data.UnitCurrency, data.TimescaleTick)
			cPriceMetric.Exact = decimal.NewFromInt64(55000)
			meas = meas.Write(bPriceMetric, cPriceMetric)

			exc, err := trainer.parseExcursion(meas)
			So(err, ShouldBeNil)
			So(exc.symbol, ShouldEqual, "BTC/USD")
			So(exc.up, ShouldBeTrue)
			So(exc.holdSeconds, ShouldEqual, 10.0)
			So(exc.gain, ShouldBeGreaterThan, 0.0)
		})

		Convey("replay walks history and computes net return and matches", func() {
			history := &recording{
				frames: []frame{{
					epoch:     100,
					at:        1000,
					symbol:    "BTC/USD",
					tokens:    [store.RegionCount]string{"R01"},
					strengths: [store.RegionCount]float64{1.0},
					bid:       50000,
					ask:       50001,
				}},
				events: []event{{
					epoch: 100,
					at:    1000,
					kind:  eventFrame,
					index: 0,
				}},
			}

			tok := &Tokenizer{Depth: 1, Regions: 1, Threshold: 0.5}
			reward, matches, _, err := history.replay(price, tok, nil)
			So(err, ShouldBeNil)
			So(matches, ShouldEqual, 0)
			So(reward, ShouldEqual, inactionPenalty)
		})

		Convey("search converges on optimal candidate", func() {
			trainer := NewTrainer(ctx, catalog, price)
			history := &recording{
				frames: make([]frame, 100),
			}

			for idx := range history.frames {
				history.frames[idx] = frame{
					epoch:  100,
					at:     int64(idx * 1000),
					symbol: "BTC/USD",
					bid:    50000 + float64(idx),
					ask:    50001 + float64(idx),
				}

				for reg := range store.RegionCount {
					history.frames[idx].tokens[reg] = "R01"
					history.frames[idx].strengths[reg] = float64(idx) / 100.0
				}

				history.events = append(history.events, event{
					epoch: 100,
					at:    int64(idx * 1000),
					kind:  eventFrame,
					index: idx,
				})
			}

			for reg := range store.RegionCount {
				history.strengths[reg] = make([]float64, 100)

				for idx := range 100 {
					history.strengths[reg][idx] = float64(idx) / 100.0
				}
			}

			history.excursions = []excursion{{
				epoch:       100,
				symbol:      "BTC/USD",
				b:           10000,
				c:           30000,
				up:          true,
				gain:        0.1,
				netReturn:   0.05,
				holdSeconds: 20.0,
			}}

			history.events = append(history.events,
				event{epoch: 100, at: 10000, kind: eventB, index: 0},
				event{epoch: 100, at: 30000, kind: eventC, index: 0},
			)

			slices.SortFunc(history.events, func(left, right event) int {
				return cmp.Or(
					cmp.Compare(left.epoch, right.epoch),
					cmp.Compare(left.at, right.at),
					cmp.Compare(left.kind, right.kind),
					cmp.Compare(left.index, right.index),
				)
			})

			best, err := trainer.search(history)
			So(err, ShouldBeNil)
			So(best, ShouldNotBeNil)
			So(best.Depth, ShouldBeGreaterThanOrEqualTo, 1)
			So(best.Regions, ShouldBeGreaterThanOrEqualTo, 1)
		})
	})
}
