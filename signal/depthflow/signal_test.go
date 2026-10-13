package depthflow_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
	nmruntime "github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/signal/depthflow"
	"github.com/theapemachine/symm/tests/market"
)

func metric(measurement *data.Measurement, label string) (float64, bool) {
	for entry := range measurement.Read(label) {
		if entry.Err != nil {
			return 0, false
		}

		return entry.Metric.Raw(), true
	}

	return 0, false
}

func metricValue(measurement *data.Measurement, label string) float64 {
	value, held := metric(measurement, label)
	So(held, ShouldBeTrue)
	return value
}

func order(id string, price, qty float64, at time.Time) kraken.Level3Order {
	return kraken.Level3Order{
		OrderID:    id,
		LimitPrice: decimal.NewFromFloat64(price),
		OrderQty:   decimal.NewFromFloat64(qty),
		Timestamp:  at,
		Event:      "add",
	}
}

func TestDepthflowSignalMetrics(t *testing.T) {
	Convey("Depthflow instrument measures exact displayed depth mutation and flow", t, func() {
		ctx := context.Background()
		books := broker.NewBook(ctx, spot.NewNormalizer())

		instrument := depthflow.NewSignal(ctx)
		instrument.Transition(nmruntime.READY)

		now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

		Convey("Level notionals, imbalances, and level-diff flow are exact", func() {
			for step := range 14 {
				at := now.Add(time.Duration(step) * 100 * time.Millisecond)

				bid1Price, bid1Qty := 50000.0, 2.0+float64(step)*0.1
				bid2Price, bid2Qty := 49990.0, 3.0+float64(step)*0.2
				ask1Price, ask1Qty := 50002.0, 1.5+float64(step)*0.1
				ask2Price, ask2Qty := 50010.0, 2.5+float64(step)*0.1

				touchBid := bid1Price * bid1Qty
				touchAsk := ask1Price * ask1Qty
				obsBid := touchBid + bid2Price*bid2Qty
				obsAsk := touchAsk + ask2Price*ask2Qty
				total := obsBid + obsAsk
				bookImb := (obsBid - obsAsk) / total

				books.Update(&kraken.Level3{
					Channel: "level3",
					Type:    "snapshot",
					Data: []kraken.Level3Data{{
						Symbol: "BTC/USD",
						Bids: []kraken.Level3Order{
							order("bid-1", bid1Price, bid1Qty, at),
							order("bid-2", bid2Price, bid2Qty, at),
						},
						Asks: []kraken.Level3Order{
							order("ask-1", ask1Price, ask1Qty, at),
							order("ask-2", ask2Price, ask2Qty, at),
						},
					}},
				})

				res := instrument.Step(market.Stamp(books, "BTC/USD", "buy", at, 1, 1))
				So(res, ShouldNotBeNil)
				So(instrument.Error(), ShouldBeNil)
				So(res.Source, ShouldEqual, "depthflow")
				So(res.Label, ShouldEqual, "BTC/USD")

				So(metricValue(res, "touch_notional:bid"), ShouldAlmostEqual, touchBid, 1e-6)
				So(metricValue(res, "book_imbalance"), ShouldAlmostEqual, bookImb, 1e-9)

				if step == 0 {
					_, held := metric(res, "net_displayed_flow:bid")
					So(held, ShouldBeFalse)
					So(res.From, ShouldEqual, at)
				}

				if step > 0 {
					So(metricValue(res, "net_displayed_flow:bid"), ShouldBeGreaterThan, 0)
					So(metricValue(res, "mutation_activity_imbalance"), ShouldBeBetweenOrEqual, -1.0, 1.0)
					So(res.From, ShouldEqual, at.Add(-100*time.Millisecond))
				}
			}
		})

		Convey("A level pushed out of a full window is not counted as removed", func() {
			levels := func(best float64, at time.Time) []kraken.Level3Order {
				bids := make([]kraken.Level3Order, 0, 10)

				for index := range 10 {
					price := best - float64(index)
					bids = append(bids, order(fmt.Sprintf("b%v", price), price, 1, at))
				}

				return bids
			}

			books.Update(&kraken.Level3{
				Channel: "level3",
				Type:    "snapshot",
				Data: []kraken.Level3Data{{
					Symbol: "ADA/USD",
					Bids:   levels(100, now),
					Asks:   []kraken.Level3Order{order("a1", 102, 1, now)},
				}},
			})
			So(instrument.Step(market.Stamp(books, "ADA/USD", "buy", now, 1, 1)), ShouldNotBeNil)

			later := now.Add(time.Second)
			books.Update(&kraken.Level3{
				Channel: "level3",
				Type:    "snapshot",
				Data: []kraken.Level3Data{{
					Symbol: "ADA/USD",
					Bids:   levels(101, later),
					Asks:   []kraken.Level3Order{order("a1", 102, 1, later)},
				}},
			})

			res := instrument.Step(market.Stamp(books, "ADA/USD", "buy", later, 1, 1))
			So(res, ShouldNotBeNil)
			So(metricValue(res, "net_displayed_flow:bid"), ShouldAlmostEqual, 101.0, 1e-9)
		})

		Convey("A vanished level counts as removed notional", func() {
			books.Update(&kraken.Level3{
				Channel: "level3",
				Type:    "snapshot",
				Data: []kraken.Level3Data{{
					Symbol: "ETH/USD",
					Bids:   []kraken.Level3Order{order("b1", 3000, 1, now), order("b2", 2999, 2, now)},
					Asks:   []kraken.Level3Order{order("a1", 3001, 1, now)},
				}},
			})
			So(instrument.Step(market.Stamp(books, "ETH/USD", "buy", now, 1, 1)), ShouldNotBeNil)

			later := now.Add(time.Second)
			books.Update(&kraken.Level3{
				Channel: "level3",
				Type:    "snapshot",
				Data: []kraken.Level3Data{{
					Symbol: "ETH/USD",
					Bids:   []kraken.Level3Order{order("b1", 3000, 1, later)},
					Asks:   []kraken.Level3Order{order("a1", 3001, 1, later)},
				}},
			})

			res := instrument.Step(market.Stamp(books, "ETH/USD", "buy", later, 1, 1))
			So(res, ShouldNotBeNil)
			So(instrument.Error(), ShouldBeNil)
			So(metricValue(res, "net_displayed_flow:bid"), ShouldAlmostEqual, -2999.0*2, 1e-6)
			So(metricValue(res, "mutation_activity_imbalance"), ShouldAlmostEqual, 1.0, 1e-9)
		})

		Convey("A crossed book is dropped with a warning without halting the system", func() {
			books.Update(&kraken.Level3{
				Channel: "level3",
				Type:    "snapshot",
				Data: []kraken.Level3Data{{
					Symbol: "XBT/USD",
					Bids:   []kraken.Level3Order{{OrderID: "bid-x", LimitPrice: decimal.NewFromFloat64(50010.0), OrderQty: decimal.NewFromFloat64(1.0), Timestamp: now, Event: "add"}},
					Asks:   []kraken.Level3Order{{OrderID: "ask-x", LimitPrice: decimal.NewFromFloat64(50000.0), OrderQty: decimal.NewFromFloat64(1.0), Timestamp: now, Event: "add"}},
				}},
			})

			So(instrument.Step(market.Stamp(books, "XBT/USD", "buy", now, 1, 1)), ShouldBeNil)
			So(instrument.Error(), ShouldBeNil)
			So(instrument.Status(), ShouldEqual, nmruntime.READY)
		})

		Convey("An absent book yields no measurement", func() {
			So(instrument.Step(market.Stamp(books, "XRP/USD", "buy", now, 1, 1)), ShouldBeNil)
			So(instrument.Error(), ShouldBeNil)
		})

		Convey("A stamped book that cannot be decoded is corrupt ingress state and halts", func() {
			So(instrument.Step(market.Unreadable("BTC/USD", "buy", now, 1, 1)), ShouldBeNil)

			err := instrument.Error()
			So(err, ShouldNotBeNil)
			So(errnie.IsInternal(err), ShouldBeTrue)
			So(err.Error(), ShouldContainSubstring, "stamped book unreadable")
			So(instrument.Status(), ShouldEqual, nmruntime.ERROR)
		})
	})
}
