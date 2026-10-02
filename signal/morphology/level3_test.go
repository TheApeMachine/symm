package morphology_test

import (
	"context"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
	nmruntime "github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/signal/morphology"
)

func TestMorphologyLevel3Metrics(t *testing.T) {
	Convey("Morphology level3 instrument computes and publishes geometric shape metrics", t, func() {
		ctx := context.Background()
		arena := data.NewArenaOwner(4096)
		normalizer := spot.NewNormalizer()
		books := broker.NewBook(ctx, normalizer)

		instrument := morphology.NewLevel3(ctx, arena, books)
		instrument.Transition(nmruntime.READY)

		now := time.Now()

		for step := 0; step < 10; step++ {
			bidPrice := decimal.NewFromFloat64(50000.0)
			askPrice := decimal.NewFromFloat64(50002.0)
			qty := decimal.NewFromFloat64(1.0 + float64(step)*0.1)

			books.Update(&kraken.Level3{
				Channel: "level3",
				Type:    "snapshot",
				Data: []kraken.Level3Data{
					{
						Symbol: "BTC/USD",
						Bids: []kraken.Level3Order{
							{
								OrderID:    "bid-1",
								LimitPrice: bidPrice,
								OrderQty:   qty,
								Timestamp:  now.Add(time.Duration(step) * 100 * time.Millisecond),
								Event:      "add",
							},
						},
						Asks: []kraken.Level3Order{
							{
								OrderID:    "ask-1",
								LimitPrice: askPrice,
								OrderQty:   qty,
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

			_, hasDist := res.LookupMetric("book_shape_distance")
			So(hasDist, ShouldBeTrue)

			_, hasKs := res.LookupMetric("book_shape_ks")
			So(hasKs, ShouldBeTrue)

			_, hasConcBid := res.LookupMetric("concentration:bid")
			So(hasConcBid, ShouldBeTrue)

			_, hasConcAsk := res.LookupMetric("concentration:ask")
			So(hasConcAsk, ShouldBeTrue)

			_, hasEntBid := res.LookupMetric("entropy:bid")
			So(hasEntBid, ShouldBeTrue)

			_, hasEntAsk := res.LookupMetric("entropy:ask")
			So(hasEntAsk, ShouldBeTrue)

			if step > 0 {
				_, hasChange := res.LookupMetric("morphology_change")
				So(hasChange, ShouldBeTrue)
			}

			if step > 2 {
				So(res.Maturity, ShouldBeGreaterThan, 0)
			}
		}
	})
}
