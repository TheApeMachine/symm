package strategy

import (
	"context"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/tests/tablestest"
)

/*
trainingFixture records one past run holding the signal tape of an excursion
with ignition B at tick 10 and peak C at tick 15, lets tape add that run's
trade or detection rows, and returns a Training for a later run that is
loading its trie.
*/
func trainingFixture(
	t *testing.T, ctx context.Context, tape func(*tables.Writer, int64),
) *Training {
	catalog := tablestest.New(t)
	epoch := int64(100)

	So(catalog.RecordRun(ctx, tables.Run{
		Epoch:     epoch,
		StartedAt: time.Now().UTC(),
		Status:    "ACTIVE",
	}), ShouldBeNil)

	normalizer := spot.NewNormalizer()
	normalizer.Update(&spot.AssetsManagerUpdate{
		NewAssets: map[string]spot.AssetInfo{
			"BTC": {AltName: "XBT"},
			"USD": {AltName: "USD"},
		},
		NewPairs: map[string]spot.AssetPair{
			"BTC/USD": {
				WSName:        "BTC/USD",
				Base:          "BTC",
				Quote:         "USD",
				LotDecimals:   8,
				LotMultiplier: 1,
			},
		},
	})

	price := broker.NewPrice(ctx, nil, nil, nil, normalizer)
	price.SetFee("BTC/USD", kraken.TradeVolumeFee{
		Fee: decimal.NewFromFloat64(0.26),
	})
	price.SetReferenceCash(decimal.NewFromFloat64(200))

	writer := tables.NewWriter(catalog, epoch)
	tape(writer, epoch)

	signal := func(source string, tick int64, seqIdx int64) {
		measurement := data.NewMeasurement(epoch, "BTC/USD", source, seqIdx, tick)
		measurement.At = time.Now().UTC()
		measurement.From = measurement.At
		valMetric := data.NewMetric("value", 1.5+float64(tick)*0.1, data.UnitCount, data.TimescaleTick)
		measurement.Write(valMetric)
		writer.Add("measurements", data.Publication{Measurement: measurement})
	}

	// Precursor A..B-1, ignition B=10, holding B+1..C-1, peak C=15.
	signal("cvd", 7, 70)
	signal("hawkes", 8, 80)
	signal("cvd", 10, 101)
	signal("depthflow", 11, 103)
	signal("liquidity", 12, 104)
	signal("cvd", 15, 105)

	So(writer.CommitReady(ctx, true), ShouldBeNil)

	storeTee := hindsight.NewStoreTee(ctx, "storeTee")
	storeTee.Transition(runtime.READY)

	training := NewTraining(
		ctx, data.NewArenaOwner("training", 4096), price, broker.NewDesk(ctx, nil, price), catalog, storeTee, 1000,
	)
	training.Transition(runtime.WAITING)
	training.Train()

	deadline := time.After(2 * time.Second)

	for training.Passes() == 0 {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for the training pass to complete")
		case <-time.After(20 * time.Millisecond):
		}
	}

	return training
}

/*
detection writes one stored detector row for the fixture excursion.
*/
func detection(lowPrice, highPrice float64) func(*tables.Writer, int64) {
	return func(writer *tables.Writer, epoch int64) {
		measurement := data.NewMeasurement(epoch, "BTC/USD", "detector", 150, 15)
		measurement.At = time.Now().UTC()
		measurement.From = measurement.At
		lowTickMetric := data.NewMetric("low_tick", 10, data.UnitCount, data.TimescaleEvent)
		highTickMetric := data.NewMetric("high_tick", 15, data.UnitCount, data.TimescaleEvent)
		lowPriceMetric := data.NewExactMetric("low_price", decimal.NewFromFloat64(lowPrice), data.UnitPrice, data.TimescaleEvent)
		highPriceMetric := data.NewExactMetric("high_price", decimal.NewFromFloat64(highPrice), data.UnitPrice, data.TimescaleEvent)
		measurement.Write(lowTickMetric, highTickMetric, lowPriceMetric, highPriceMetric)
		writer.Add("measurements", data.Publication{Measurement: measurement})
	}
}

func TestTraining_Train(t *testing.T) {
	Convey("Given a past run with a stored excursion that clears friction", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := trainingFixture(t, ctx, detection(60000, 63000))

		Convey("It loads enter and exit associations into the trie and opens trading", func() {
			So(training.Status(), ShouldEqual, runtime.READY)
			So(training.records(), ShouldBeGreaterThan, 0)
			So(training.classCount("enter"), ShouldBeGreaterThan, 0)
			So(training.classCount("exit"), ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given a past run with a trade tape that does not clear friction", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := trainingFixture(t, ctx, func(writer *tables.Writer, epoch int64) {
			for tick, price := range map[int64]string{
				10: "60000", 11: "60005", 15: "60010", 16: "60008",
			} {
				exact, err := decimal.NewFromString(price)
				So(err, ShouldBeNil)

				trade := data.NewMeasurement(epoch, "BTC/USD", "spot:trade", tick, tick)
				trade.At = time.Now().UTC()
				trade.From = trade.At
				priceMetric := data.NewMetric("price", exact.Float64(), data.UnitPrice, data.TimescaleTick)
				priceMetric.Exact = exact
				trade.Write(priceMetric)
				writer.Add("measurements", data.Publication{Measurement: trade})
			}
		})

		Convey("It remains in WAITING without training on the losing excursion", func() {
			So(training.Status(), ShouldEqual, runtime.WAITING)
			So(training.records(), ShouldEqual, 0)
		})
	})

	Convey("Given a past run with only its trade tape stored", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := trainingFixture(t, ctx, func(writer *tables.Writer, epoch int64) {
			// The high at tick 6 precedes the low; the draw-up is 10 -> 15.
			for tick, price := range map[int64]string{
				6: "64000", 7: "61000", 8: "60500", 10: "60000",
				11: "61000", 12: "62000", 15: "63000", 16: "62500",
			} {
				exact, err := decimal.NewFromString(price)
				So(err, ShouldBeNil)

				trade := data.NewMeasurement(epoch, "BTC/USD", "spot:trade", tick, tick)
				trade.At = time.Now().UTC()
				trade.From = trade.At
				priceMetric := data.NewMetric("price", exact.Float64(), data.UnitPrice, data.TimescaleTick)
				priceMetric.Exact = exact
				trade.Write(priceMetric)
				writer.Add("measurements", data.Publication{Measurement: trade})
			}
		})

		Convey("It detects the excursion and learns from it in the same pass", func() {
			So(training.Status(), ShouldEqual, runtime.READY)
			So(training.classCount("enter"), ShouldBeGreaterThan, 0)
			So(training.classCount("exit"), ShouldBeGreaterThan, 0)
		})
	})
}

func TestRegionToken(t *testing.T) {
	Convey("Given training component and measurements", t, func() {
		training := &Training{
			grid: store.NewGrid(),
		}

		tok := training.token()
		So(tok, ShouldBeNil)
	})
}

func TestTraining_Step(t *testing.T) {
	Convey("Given a Training instance with an off-ramp worker", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		catalog := tablestest.New(t)
		normalizer := spot.NewNormalizer()
		price := broker.NewPrice(ctx, nil, nil, nil, normalizer)
		desk := broker.NewDesk(ctx, nil, price)
		storeTee := hindsight.NewStoreTee(ctx, "storeTee")
		storeTee.Transition(runtime.READY)

		training := NewTraining(
			ctx, data.NewArenaOwner("training", 4096), price, desk, catalog, storeTee, 1000,
		)

		testUITee := hindsight.NewStoreTee(ctx, "uiTee")
		testUITee.Transition(runtime.READY)
		training.SetUITee(testUITee)

		Convey("Step processes measurement synchronously and returns report", func() {
			measurement := data.NewMeasurement(1000, "BTC/USD", "cvd", 1, 1)
			measurement.At = time.Now().UTC()
			measurement.From = measurement.At
			priceMetric := data.NewMetric("price", 60000, data.UnitPrice, data.TimescaleTick)
			measurement.Write(priceMetric)

			result := training.Step(measurement)
			So(result, ShouldNotBeNil)
			So(result.Source, ShouldEqual, "training")
			So(result.Label, ShouldEqual, "BTC/USD")
		})
	})
}

func TestTraining_TriadGate(t *testing.T) {
	Convey("Given a Training component with triad gating", t, func() {
		training := &Training{}

		Convey("When resonance surprise is positive and manifold impedance is clear", func() {
			resonanceM := data.NewMeasurement(1, "BTC/USD", "resonance", 1, 1)
			resonanceM.Write(data.NewMetric("surprise", 1.5, data.UnitRatio, data.TimescaleInstantaneous))

			manifoldM := data.NewMeasurement(1, "BTC/USD", "manifold", 1, 1)
			manifoldM.Write(
				data.NewMetric("kuramoto_r", 0.4, data.UnitRatio, data.TimescaleInstantaneous),
				data.NewMetric("pressure_grad_norm", 0.1, data.UnitRatio, data.TimescaleInstantaneous),
			)

			So(training.authorized(resonanceM, manifoldM), ShouldBeTrue)
		})

		Convey("When resonance surprise is zero (equilibrium churn), entry is vetoed", func() {
			resonanceM := data.NewMeasurement(1, "BTC/USD", "resonance", 1, 1)
			resonanceM.Write(data.NewMetric("surprise", 0.0, data.UnitRatio, data.TimescaleInstantaneous))

			manifoldM := data.NewMeasurement(1, "BTC/USD", "manifold", 1, 1)
			manifoldM.Write(data.NewMetric("kuramoto_r", 0.4, data.UnitRatio, data.TimescaleInstantaneous))

			So(training.authorized(resonanceM, manifoldM), ShouldBeFalse)
		})

		Convey("When manifold has complete locked synchronization and opposing pressure, entry is vetoed", func() {
			resonanceM := data.NewMeasurement(1, "BTC/USD", "resonance", 1, 1)
			resonanceM.Write(data.NewMetric("surprise", 2.0, data.UnitRatio, data.TimescaleInstantaneous))

			manifoldM := data.NewMeasurement(1, "BTC/USD", "manifold", 1, 1)
			manifoldM.Write(
				data.NewMetric("kuramoto_r", 1.0, data.UnitRatio, data.TimescaleInstantaneous),
				data.NewMetric("pressure_grad_norm", 5.0, data.UnitRatio, data.TimescaleInstantaneous),
			)

			So(training.authorized(resonanceM, manifoldM), ShouldBeFalse)
		})
	})
}
