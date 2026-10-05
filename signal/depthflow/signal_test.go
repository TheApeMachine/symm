package depthflow_test

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
	nmruntime "github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/signal/depthflow"
)

func TestDepthflowSignalMetrics(t *testing.T) {
	Convey("Depthflow signal instrument calculates exact displayed depth mutation and flow metrics", t, func() {
		ctx := context.Background()
		arena := data.NewArenaOwner(4096)
		normalizer := spot.NewNormalizer()
		books := broker.NewBook(ctx, normalizer)

		instrument := depthflow.NewSignal(ctx, arena, books)
		instrument.Transition(nmruntime.READY)

		now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

		Convey("Computes exact level notionals, book imbalance, touch imbalance, and resolution gap", func() {
			for step := 0; step < 10; step++ {
				// 2 bid levels and 2 ask levels
				bid1Price := 50000.0
				bid1Qty := 2.0 + float64(step)*0.1
				bid2Price := 49990.0
				bid2Qty := 3.0 + float64(step)*0.2

				ask1Price := 50002.0
				ask1Qty := 1.5 + float64(step)*0.1
				ask2Price := 50010.0
				ask2Qty := 2.5 + float64(step)*0.1

				expectedTouchBidNotional := bid1Price * bid1Qty
				expectedObsBid := expectedTouchBidNotional + bid2Price*bid2Qty

				expectedTouchAskNotional := ask1Price * ask1Qty
				expectedObsAsk := expectedTouchAskNotional + ask2Price*ask2Qty

				expectedTotalNotional := expectedObsBid + expectedObsAsk
				expectedBookImb := (expectedObsBid - expectedObsAsk) / expectedTotalNotional
				expectedTouchImb := (expectedTouchBidNotional - expectedTouchAskNotional) / (expectedTouchBidNotional + expectedTouchAskNotional)
				expectedGap := expectedTouchImb - expectedBookImb
				expectedDist := math.Abs(expectedGap)

				books.Update(&kraken.Level3{
					Channel: "level3",
					Type:    "snapshot",
					Data: []kraken.Level3Data{
						{
							Symbol: "BTC/USD",
							Bids: []kraken.Level3Order{
								{
									OrderID:    "bid-1",
									LimitPrice: decimal.NewFromFloat64(bid1Price),
									OrderQty:   decimal.NewFromFloat64(bid1Qty),
									Timestamp:  now.Add(time.Duration(step) * 100 * time.Millisecond),
									Event:      "add",
								},
								{
									OrderID:    "bid-2",
									LimitPrice: decimal.NewFromFloat64(bid2Price),
									OrderQty:   decimal.NewFromFloat64(bid2Qty),
									Timestamp:  now.Add(time.Duration(step) * 100 * time.Millisecond),
									Event:      "add",
								},
							},
							Asks: []kraken.Level3Order{
								{
									OrderID:    "ask-1",
									LimitPrice: decimal.NewFromFloat64(ask1Price),
									OrderQty:   decimal.NewFromFloat64(ask1Qty),
									Timestamp:  now.Add(time.Duration(step) * 100 * time.Millisecond),
									Event:      "add",
								},
								{
									OrderID:    "ask-2",
									LimitPrice: decimal.NewFromFloat64(ask2Price),
									OrderQty:   decimal.NewFromFloat64(ask2Qty),
									Timestamp:  now.Add(time.Duration(step) * 100 * time.Millisecond),
									Event:      "add",
								},
							},
						},
					},
				})

				prior := arena.NewMeasurement("ingress")
				prior.Label = "BTC/USD"
				prior.SeqIdx = int64(step + 1)
				prior.At = now.Add(time.Duration(step) * 100 * time.Millisecond)
				prior.From = prior.At

				res := instrument.Step(prior)
				So(res, ShouldNotBeNil)
				So(res.Err, ShouldBeNil)

				// Level notionals
				So(res.GetMetric("book_notional:bid").Raw, ShouldAlmostEqual, expectedObsBid, 1e-6)
				So(res.GetMetric("book_notional:ask").Raw, ShouldAlmostEqual, expectedObsAsk, 1e-6)
				So(res.GetMetric("book_notional").Raw, ShouldAlmostEqual, expectedTotalNotional, 1e-6)
				So(res.GetMetric("observed_notional").Raw, ShouldAlmostEqual, expectedTotalNotional, 1e-6)

				// Imbalances
				So(res.GetMetric("book_imbalance").Raw, ShouldAlmostEqual, expectedBookImb, 1e-9)
				So(res.GetMetric("touch_imbalance").Raw, ShouldAlmostEqual, expectedTouchImb, 1e-9)
				So(res.GetMetric("imbalance_resolution_gap").Raw, ShouldAlmostEqual, expectedGap, 1e-9)
				So(res.GetMetric("imbalance_resolution_distance").Raw, ShouldAlmostEqual, expectedDist, 1e-9)

				// Flow additions / removals between steps
				if step > 0 {
					So(res.GetMetric("added_notional:bid").Raw, ShouldBeGreaterThan, 0)
					So(res.GetMetric("added_notional:ask").Raw, ShouldBeGreaterThan, 0)
				}

				if step > 2 {
					So(res.Maturity, ShouldBeGreaterThan, 0)
					So(res.SNRDefined, ShouldBeTrue)
				}
			}
		})
	})
}
