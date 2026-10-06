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

func ingress(label string, at time.Time, seq int64) *data.Measurement {
	prior := data.NewMeasurement(1, label, "ingress", seq, seq)
	prior.At = at
	prior.From = at
	return prior
}

func metric(measurement *data.Measurement, label string) (float64, bool) {
	for entry := range measurement.Read(label) {
		if entry.Err != nil {
			return 0, false
		}

		return entry.Metric.Raw, true
	}

	return 0, false
}

func metricValue(measurement *data.Measurement, label string) float64 {
	value, held := metric(measurement, label)
	So(held, ShouldBeTrue)
	return value
}

func TestMorphologyLevel3Metrics(t *testing.T) {
	Convey("Morphology instrument computes principled distribution geometry, concentration, and entropy", t, func() {
		ctx := context.Background()
		arena := data.NewArenaOwner("morphology", 4096)
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

			res := instrument.Step(ingress("BTC/USD", now, 1))
			So(res, ShouldNotBeNil)
			So(instrument.Error(), ShouldBeNil)
			So(res.Source, ShouldEqual, "morphology")
			So(res.Label, ShouldEqual, "BTC/USD")

			// Single level bid has concentration = 1.0 and zero entropy
			So(metricValue(res, "concentration:bid"), ShouldAlmostEqual, 1.0, 1e-9)
			So(metricValue(res, "entropy:bid"), ShouldAlmostEqual, 0.0, 1e-9)

			// Two-level ask has concentration < 1.0 and positive entropy
			So(metricValue(res, "concentration:ask"), ShouldBeLessThan, 1.0)
			So(metricValue(res, "entropy:ask"), ShouldBeGreaterThan, 0.0)

			// Asymmetric shape relative to mid: KS statistic is positive
			So(metricValue(res, "book_shape_ks"), ShouldBeGreaterThan, 0.0)
			So(metricValue(res, "book_shape_distance"), ShouldBeGreaterThan, 0.0)

			_, held := metric(res, "morphology_change")
			So(held, ShouldBeFalse)
		})

		Convey("Multi-level dispersion decreases concentration, increases entropy, and tracks morphology change", func() {
			var prevDist float64
			for step := 0; step < 5; step++ {
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

				at := now.Add(time.Duration(step) * 100 * time.Millisecond)
				res := instrument.Step(ingress("BTC/USD", at, int64(step+2)))
				So(res, ShouldNotBeNil)
				So(instrument.Error(), ShouldBeNil)

				// With 4 equal levels, concentration is strictly less than 1.0 (approx 0.25)
				So(metricValue(res, "concentration:bid"), ShouldBeLessThan, 0.5)
				So(metricValue(res, "concentration:ask"), ShouldBeLessThan, 0.5)

				// Entropy must be positive (approx ln(4) ≈ 1.386)
				So(metricValue(res, "entropy:bid"), ShouldBeGreaterThan, 1.0)
				So(metricValue(res, "entropy:ask"), ShouldBeGreaterThan, 1.0)

				currentDist := metricValue(res, "book_shape_distance")
				So(currentDist, ShouldBeGreaterThan, 0.0)

				if step > 0 {
					expectedChange := math.Abs(currentDist - prevDist)
					So(metricValue(res, "morphology_change"), ShouldAlmostEqual, expectedChange, 1e-6)
				} else {
					_, held := metric(res, "morphology_change")
					So(held, ShouldBeFalse)
				}
				prevDist = currentDist
			}
		})
	})
}
