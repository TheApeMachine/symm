package morphology_test

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
	"github.com/theapemachine/symm/signal/morphology"
)

func TestMorphologyLevel3Metrics(t *testing.T) {
	Convey("Morphology instrument computes principled distribution geometry, concentration, and entropy", t, func() {
		ctx := context.Background()
		arena := data.NewArenaOwner(4096)
		normalizer := spot.NewNormalizer()
		books := broker.NewBook(ctx, normalizer)

		instrument := morphology.NewSignal(ctx, arena, books)
		instrument.Transition(nmruntime.READY)

		now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

		Convey("Asymmetric depth distribution yields exact KS distance and Wasserstein shape separation", func() {
			books.Update(&kraken.Level3{
				Channel: "level3",
				Type:    "snapshot",
				Data: []kraken.Level3Data{
					{
						Symbol: "BTC/USD",
						Bids: []kraken.Level3Order{
							{
								OrderID:    "bid-1",
								LimitPrice: decimal.NewFromFloat64(50000.0),
								OrderQty:   decimal.NewFromFloat64(2.0),
								Timestamp:  now,
								Event:      "add",
							},
						},
						Asks: []kraken.Level3Order{
							{
								OrderID:    "ask-1",
								LimitPrice: decimal.NewFromFloat64(50010.0),
								OrderQty:   decimal.NewFromFloat64(1.0),
								Timestamp:  now,
								Event:      "add",
							},
							{
								OrderID:    "ask-2",
								LimitPrice: decimal.NewFromFloat64(50020.0),
								OrderQty:   decimal.NewFromFloat64(1.0),
								Timestamp:  now,
								Event:      "add",
							},
						},
					},
				},
			})

			prior := arena.NewMeasurement("ingress")
			prior.Label = "BTC/USD"
			prior.SeqIdx = 1
			prior.At = now
			prior.From = now

			res := instrument.Step(prior)
			So(res, ShouldNotBeNil)
			So(res.Err, ShouldBeNil)

			// Single level bid has concentration = 1.0 and zero entropy
			So(res.GetMetric("concentration:bid").Raw, ShouldAlmostEqual, 1.0, 1e-9)
			So(res.GetMetric("entropy:bid").Raw, ShouldAlmostEqual, 0.0, 1e-9)

			// Two-level ask has concentration < 1.0 and positive entropy
			So(res.GetMetric("concentration:ask").Raw, ShouldBeLessThan, 1.0)
			So(res.GetMetric("entropy:ask").Raw, ShouldBeGreaterThan, 0.0)

			// Asymmetric shape relative to mid: KS statistic is positive
			So(res.GetMetric("book_shape_ks").Raw, ShouldBeGreaterThan, 0.0)
			So(res.GetMetric("book_shape_distance").Raw, ShouldBeGreaterThan, 0.0)
		})

		Convey("Multi-level dispersion decreases concentration, increases entropy, and tracks morphology change", func() {
			var prevDist float64
			for step := 0; step < 5; step++ {
				// 4 levels of bids and asks
				var bids, asks []kraken.Level3Order
				for level := 0; level < 4; level++ {
					bids = append(bids, kraken.Level3Order{
						OrderID:    "bid-" + string(rune('a'+level)),
						LimitPrice: decimal.NewFromFloat64(50000.0 - float64(level)*10.0),
						OrderQty:   decimal.NewFromFloat64(1.0),
						Timestamp:  now.Add(time.Duration(step) * 100 * time.Millisecond),
						Event:      "add",
					})
					asks = append(asks, kraken.Level3Order{
						OrderID:    "ask-" + string(rune('a'+level)),
						LimitPrice: decimal.NewFromFloat64(50020.0 + float64(level)*10.0 + float64(step)*5.0),
						OrderQty:   decimal.NewFromFloat64(1.0),
						Timestamp:  now.Add(time.Duration(step) * 100 * time.Millisecond),
						Event:      "add",
					})
				}

				books.Update(&kraken.Level3{
					Channel: "level3",
					Type:    "snapshot",
					Data: []kraken.Level3Data{
						{
							Symbol: "BTC/USD",
							Bids:   bids,
							Asks:   asks,
						},
					},
				})

				prior := arena.NewMeasurement("ingress")
				prior.Label = "BTC/USD"
				prior.SeqIdx = int64(step + 2)
				prior.At = now.Add(time.Duration(step) * 100 * time.Millisecond)
				prior.From = prior.At

				res := instrument.Step(prior)
				So(res, ShouldNotBeNil)
				So(res.Err, ShouldBeNil)

				// With 4 equal levels, concentration is strictly less than 1.0 (approx 0.25)
				So(res.GetMetric("concentration:bid").Raw, ShouldBeLessThan, 0.5)
				So(res.GetMetric("concentration:ask").Raw, ShouldBeLessThan, 0.5)

				// Entropy must be positive (approx ln(4) ≈ 1.386)
				So(res.GetMetric("entropy:bid").Raw, ShouldBeGreaterThan, 1.0)
				So(res.GetMetric("entropy:ask").Raw, ShouldBeGreaterThan, 1.0)

				currentDist := res.GetMetric("book_shape_distance").Raw
				So(currentDist, ShouldBeGreaterThan, 0.0)

				if step > 0 {
					expectedChange := math.Abs(currentDist - prevDist)
					So(res.GetMetric("morphology_change").Raw, ShouldAlmostEqual, expectedChange, 1e-6)
				}
				prevDist = currentDist
			}
		})
	})
}
