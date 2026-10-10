package depthflow_test

import (
	"context"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	nmruntime "github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/signal/depthflow"
)

func TestDepthflowReadsBookAsOfFrame(t *testing.T) {
	Convey("Given frames processed after the live book has moved on", t, func() {
		ctx := context.Background()
		books := broker.NewBook(ctx, spot.NewNormalizer())
		instrument := depthflow.NewSignal(ctx, books)
		instrument.Transition(nmruntime.READY)
		origin := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
		at := func(step int) time.Time { return origin.Add(time.Duration(step) * 100 * time.Millisecond) }
		bidQty := func(step int) float64 { return 1 + float64(step) }

		apply := func(step int) {
			books.Update(&kraken.Level3{
				Channel: "level3",
				Type:    "snapshot",
				Data: []kraken.Level3Data{{
					Symbol: "BTC/USD",
					Bids:   []kraken.Level3Order{order("bid", 50000, bidQty(step), at(step))},
					Asks:   []kraken.Level3Order{order("ask", 50002, 1, at(step))},
				}},
			})
		}

		// The pipeline lag is learned from lookups: one frame 400ms behind the
		// live book teaches the history to keep that much.
		for step := 0; step <= 4; step++ {
			apply(step)
		}

		books.BookAt("BTC/USD", at(0), func(*broker.BookView) {})

		for step := 5; step <= 9; step++ {
			apply(step)
		}

		Convey("Each frame measures the book it was stamped against", func() {
			for step := 5; step <= 9; step++ {
				res := instrument.Step(ingress("BTC/USD", at(step), int64(step)))
				So(res, ShouldNotBeNil)
				So(metricValue(res, "book_notional:bid"), ShouldAlmostEqual, 50000*bidQty(step), 1e-6)
			}
		})
	})
}
