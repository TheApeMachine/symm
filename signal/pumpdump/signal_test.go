package pumpdump_test

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
	"github.com/theapemachine/symm/signal/pumpdump"
)

func TestPumpDumpSignal(t *testing.T) {
	Convey("Pumpdump / Volume-Clocked Activity instrument computes principled market anomaly metrics", t, func() {
		ctx := context.Background()
		arena := data.NewArenaOwner(4096)
		now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

		normalizer := spot.NewNormalizer()
		books := broker.NewBook(ctx, normalizer)

		instrument := pumpdump.NewSignal(ctx, arena, books)
		instrument.Transition(nmruntime.READY)

		Convey("Volume clock aggregates exact trade bars, durations, and volume/notional rates", func() {
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
								OrderQty:   decimal.NewFromFloat64(5.0),
								Timestamp:  now,
								Event:      "add",
							},
						},
						Asks: []kraken.Level3Order{
							{
								OrderID:    "ask-1",
								LimitPrice: decimal.NewFromFloat64(50002.0),
								OrderQty:   decimal.NewFromFloat64(5.0),
								Timestamp:  now,
								Event:      "add",
							},
						},
					},
				},
			})

			// Trade 1: establishes start time and initial target quantity
			prior1 := arena.NewMeasurement("ingress")
			prior1.Label = "BTC/USD"
			prior1.SeqIdx = 1
			prior1.At = now
			prior1.From = now
			prior1.SetMetric("price", data.NewMetric("price", data.UnitPrice, data.TimescaleInstantaneous, 50001.0, 1.0).Write(50001.0))
			prior1.SetMetric("qty", data.NewMetric("qty", data.UnitQuantity, data.TimescaleInstantaneous, 0.0, 1.0).Write(1.0))
			prior1.SetProvenance("channel", "trade")
			prior1.SetProvenance("side", "buy")

			res1 := instrument.Step(prior1)
			So(res1, ShouldNotBeNil)
			So(res1.Err, ShouldBeNil)
			So(res1.GetMetric("trade_price").Raw, ShouldEqual, 50001.0)
			So(res1.GetMetric("trade_quantity").Raw, ShouldEqual, 1.0)
			So(res1.GetMetric("trade_notional").Raw, ShouldEqual, 50001.0)

			// Trade 2: 200ms later, qty=1.0. Accumulates barQty=2.0 >= targetQty (1.0). Bar completes!
			trade2At := now.Add(200 * time.Millisecond)
			prior2 := arena.NewMeasurement("ingress")
			prior2.Label = "BTC/USD"
			prior2.SeqIdx = 2
			prior2.At = trade2At
			prior2.From = trade2At
			prior2.SetMetric("price", data.NewMetric("price", data.UnitPrice, data.TimescaleInstantaneous, 50002.0, 1.0).Write(50002.0))
			prior2.SetMetric("qty", data.NewMetric("qty", data.UnitQuantity, data.TimescaleInstantaneous, 0.0, 1.0).Write(1.0))
			prior2.SetProvenance("channel", "trade")
			prior2.SetProvenance("side", "buy")

			res2 := instrument.Step(prior2)
			So(res2, ShouldNotBeNil)
			So(res2.Err, ShouldBeNil)

			// Exact volume bar metrics
			So(res2.GetMetric("volume_bar_quantity").Raw, ShouldAlmostEqual, 2.0, 1e-9)
			So(res2.GetMetric("volume_bar_notional").Raw, ShouldAlmostEqual, 50001.0+50002.0, 1e-6)
			So(res2.GetMetric("volume_bar_trade_count").Raw, ShouldEqual, 2)
			So(res2.GetMetric("volume_bar_duration").Raw, ShouldAlmostEqual, 0.2, 1e-6)
			So(res2.GetMetric("volume_rate").Raw, ShouldAlmostEqual, 2.0/0.2, 1e-6)
			So(res2.GetMetric("notional_rate").Raw, ShouldAlmostEqual, 100003.0/0.2, 1e-6)
			So(res2.GetMetric("trade_rate").Raw, ShouldAlmostEqual, 2.0/0.2, 1e-6)
			So(res2.GetMetric("completed_bars").Raw, ShouldEqual, 1)
		})

		Convey("Detects pump anomaly: explosive volume surge and spread blowout trigger positive divergences and z-score", func() {
			pumpInstrument := pumpdump.NewSignal(ctx, arena, books)
			pumpInstrument.Transition(nmruntime.READY)

			// Step 1: Establish baseline with 12 small calm trades with tight spread
			basePrice := 50000.0
			for step := 0; step < 12; step++ {
				books.Update(&kraken.Level3{
					Channel: "level3",
					Type:    "snapshot",
					Data: []kraken.Level3Data{
						{
							Symbol: "BTC/USD",
							Bids: []kraken.Level3Order{
								{
									OrderID:    "bid-calm",
									LimitPrice: decimal.NewFromFloat64(basePrice - 1.0),
									OrderQty:   decimal.NewFromFloat64(5.0),
									Timestamp:  now.Add(time.Duration(step*100) * time.Millisecond),
									Event:      "add",
								},
							},
							Asks: []kraken.Level3Order{
								{
									OrderID:    "ask-calm",
									LimitPrice: decimal.NewFromFloat64(basePrice + 1.0),
									OrderQty:   decimal.NewFromFloat64(5.0),
									Timestamp:  now.Add(time.Duration(step*100) * time.Millisecond),
									Event:      "add",
								},
							},
						},
					},
				})

				prior := arena.NewMeasurement("ingress")
				prior.Label = "BTC/USD"
				prior.SeqIdx = int64(step + 10)
				prior.At = now.Add(time.Duration(step*100) * time.Millisecond)
				prior.From = prior.At
				prior.SetMetric("price", data.NewMetric("price", data.UnitPrice, data.TimescaleInstantaneous, basePrice, 1.0).Write(basePrice))
				prior.SetMetric("qty", data.NewMetric("qty", data.UnitQuantity, data.TimescaleInstantaneous, 0.0, 0.5).Write(0.5))
				prior.SetProvenance("channel", "trade")
				prior.SetProvenance("side", "buy")

				pumpInstrument.Step(prior)
			}

			// Step 2: Explosive pump event: spread blows out 20x and massive trade volume arrives
			pumpAt := now.Add(1300 * time.Millisecond)
			books.Update(&kraken.Level3{
				Channel: "level3",
				Type:    "snapshot",
				Data: []kraken.Level3Data{
					{
						Symbol: "BTC/USD",
						Bids: []kraken.Level3Order{
							{
								OrderID:    "bid-pump",
								LimitPrice: decimal.NewFromFloat64(basePrice),
								OrderQty:   decimal.NewFromFloat64(1.0),
								Timestamp:  pumpAt,
								Event:      "add",
							},
						},
						Asks: []kraken.Level3Order{
							{
								OrderID:    "ask-pump",
								LimitPrice: decimal.NewFromFloat64(basePrice + 40.0), // Massive spread blowout
								OrderQty:   decimal.NewFromFloat64(1.0),
								Timestamp:  pumpAt,
								Event:      "add",
							},
						},
					},
				},
			})

			pumpPrior := arena.NewMeasurement("ingress")
			pumpPrior.Label = "BTC/USD"
			pumpPrior.SeqIdx = 25
			pumpPrior.At = pumpAt
			pumpPrior.From = pumpAt
			pumpPrior.SetMetric("price", data.NewMetric("price", data.UnitPrice, data.TimescaleInstantaneous, basePrice+35.0, 1.0).Write(basePrice+35.0))
			pumpPrior.SetMetric("qty", data.NewMetric("qty", data.UnitQuantity, data.TimescaleInstantaneous, 0.0, 10.0).Write(10.0)) // 20x volume
			pumpPrior.SetProvenance("channel", "trade")
			pumpPrior.SetProvenance("side", "buy")

			pumpRes := pumpInstrument.Step(pumpPrior)
			So(pumpRes, ShouldNotBeNil)

			// Statistical divergence and anomaly metrics
			spreadDivMetric, hasDiv := pumpRes.LookupMetric("spread_divergence")
			So(hasDiv, ShouldBeTrue)
			So(spreadDivMetric.Raw, ShouldBeGreaterThan, 0.0) // Positive spread blowout divergence

			zscoreMetric, hasZ := pumpRes.LookupMetric("spread_zscore")
			So(hasZ, ShouldBeTrue)
			So(zscoreMetric.Raw, ShouldBeGreaterThan, 2.0) // Statistical outlier z-score > 2.0

			notionalDivMetric, hasNotionalDiv := pumpRes.LookupMetric("notional_rate_divergence")
			So(hasNotionalDiv, ShouldBeTrue)
			So(notionalDivMetric.Raw, ShouldBeGreaterThan, 0.0)
		})

		Convey("Detects dump anomaly: heavy sell trade under price crash captures negative midpoint return", func() {
			dumpInstrument := pumpdump.NewSignal(ctx, arena, books)
			dumpInstrument.Transition(nmruntime.READY)

			// Calm baseline
			basePrice := 50000.0
			for step := 0; step < 5; step++ {
				books.Update(&kraken.Level3{
					Channel: "level3",
					Type:    "snapshot",
					Data: []kraken.Level3Data{
						{
							Symbol: "BTC/USD",
							Bids: []kraken.Level3Order{
								{
									OrderID:    "bid-calm",
									LimitPrice: decimal.NewFromFloat64(basePrice - 2.0),
									OrderQty:   decimal.NewFromFloat64(5.0),
									Timestamp:  now.Add(time.Duration(step*100) * time.Millisecond),
									Event:      "add",
								},
							},
							Asks: []kraken.Level3Order{
								{
									OrderID:    "ask-calm",
									LimitPrice: decimal.NewFromFloat64(basePrice + 2.0),
									OrderQty:   decimal.NewFromFloat64(5.0),
									Timestamp:  now.Add(time.Duration(step*100) * time.Millisecond),
									Event:      "add",
								},
							},
						},
					},
				})

				prior := arena.NewMeasurement("ingress")
				prior.Label = "BTC/USD"
				prior.SeqIdx = int64(step + 30)
				prior.At = now.Add(time.Duration(step*100) * time.Millisecond)
				prior.From = prior.At
				prior.SetMetric("price", data.NewMetric("price", data.UnitPrice, data.TimescaleInstantaneous, basePrice, 1.0).Write(basePrice))
				prior.SetMetric("qty", data.NewMetric("qty", data.UnitQuantity, data.TimescaleInstantaneous, 0.0, 1.0).Write(1.0))
				prior.SetProvenance("channel", "trade")
				prior.SetProvenance("side", "sell")

				dumpInstrument.Step(prior)
			}

			// Sharp dump: price drops by 50 points under selling
			dumpAt := now.Add(600 * time.Millisecond)
			books.Update(&kraken.Level3{
				Channel: "level3",
				Type:    "snapshot",
				Data: []kraken.Level3Data{
					{
						Symbol: "BTC/USD",
						Bids: []kraken.Level3Order{
							{
								OrderID:    "bid-dump",
								LimitPrice: decimal.NewFromFloat64(basePrice - 52.0),
								OrderQty:   decimal.NewFromFloat64(5.0),
								Timestamp:  dumpAt,
								Event:      "add",
							},
						},
						Asks: []kraken.Level3Order{
							{
								OrderID:    "ask-dump",
								LimitPrice: decimal.NewFromFloat64(basePrice - 48.0),
								OrderQty:   decimal.NewFromFloat64(5.0),
								Timestamp:  dumpAt,
								Event:      "add",
							},
						},
					},
				},
			})

			dumpPrior := arena.NewMeasurement("ingress")
			dumpPrior.Label = "BTC/USD"
			dumpPrior.SeqIdx = 36
			dumpPrior.At = dumpAt
			dumpPrior.From = dumpAt
			dumpPrior.SetMetric("price", data.NewMetric("price", data.UnitPrice, data.TimescaleInstantaneous, basePrice-50.0, 1.0).Write(basePrice-50.0))
			dumpPrior.SetMetric("qty", data.NewMetric("qty", data.UnitQuantity, data.TimescaleInstantaneous, 0.0, 3.0).Write(3.0))
			dumpPrior.SetProvenance("channel", "trade")
			dumpPrior.SetProvenance("side", "sell")

			dumpRes := dumpInstrument.Step(dumpPrior)
			So(dumpRes, ShouldNotBeNil)

			negReturnMetric, hasNeg := dumpRes.LookupMetric("negative_midpoint_return")
			So(hasNeg, ShouldBeTrue)
			So(negReturnMetric.Raw, ShouldBeLessThan, 0.0)

			returnRateMetric, hasRate := dumpRes.LookupMetric("midpoint_return_rate")
			So(hasRate, ShouldBeTrue)
			So(returnRateMetric.Raw, ShouldBeLessThan, 0.0)
		})

		Convey("Rejects crossed order book quotes with explicit internal error", func() {
			crossedInstrument := pumpdump.NewSignal(ctx, arena, books)
			crossedInstrument.Transition(nmruntime.READY)

			books.Update(&kraken.Level3{
				Channel: "level3",
				Type:    "snapshot",
				Data: []kraken.Level3Data{
					{
						Symbol: "BTC/USD",
						Bids: []kraken.Level3Order{
							{
								OrderID:    "bid-crossed",
								LimitPrice: decimal.NewFromFloat64(50010.0),
								OrderQty:   decimal.NewFromFloat64(1.0),
								Timestamp:  now,
								Event:      "add",
							},
						},
						Asks: []kraken.Level3Order{
							{
								OrderID:    "ask-crossed",
								LimitPrice: decimal.NewFromFloat64(50000.0), // crossed: ask < bid
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
			prior.SeqIdx = 50
			prior.At = now
			prior.From = now
			prior.SetMetric("price", data.NewMetric("price", data.UnitPrice, data.TimescaleInstantaneous, 50005.0, 1.0).Write(50005.0))
			prior.SetMetric("qty", data.NewMetric("qty", data.UnitQuantity, data.TimescaleInstantaneous, 0.0, 1.0).Write(1.0))

			res := crossedInstrument.Step(prior)
			So(res, ShouldNotBeNil)
			So(res.Err, ShouldNotBeNil)
		})
	})
}
