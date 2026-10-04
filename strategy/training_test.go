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

	defVal := 0.5
	signal := func(source string, tick int64, seqIdx int64) {
		measurement := data.NewMeasurement[float64](source, nil)
		measurement.Epoch = epoch
		measurement.Label = "BTC/USD"
		measurement.Tick = tick
		measurement.SeqIdx = seqIdx
		measurement.Maturity = 1.0
		measurement.SetMetric("value", data.Metric[float64]{Label: "value", Raw: 1.5, Deformation: &defVal})
		writer.Add("measurements", data.Publication{Measurement: measurement})
	}

	// Precursor A..B-1, ignition B=10, holding B+1..C-1, peak C=15.
	signal("cvd", 7, 70)
	signal("hawkes", 8, 80)
	signal("cvd", 10, 101)
	signal("resonance", 11, 103)
	signal("liquidity", 12, 104)
	signal("manifold", 15, 105)

	So(writer.CommitReady(ctx, true), ShouldBeNil)

	storeTee := hindsight.NewStoreTee(ctx, "storeTee")
	storeTee.Transition(runtime.READY)

	training := NewTraining(
		ctx, data.NewArenaOwner(4096), price, broker.NewDesk(ctx, nil, price), catalog, storeTee, 1000,
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
		measurement := data.NewMeasurement[float64]("detector", nil)
		measurement.Epoch = epoch
		measurement.Label = "BTC/USD"
		measurement.Tick = 15
		measurement.SeqIdx = 150
		measurement.SetMetric("LowTick", data.Metric[float64]{Raw: 10})
		measurement.SetMetric("HighTick", data.Metric[float64]{Raw: 15})
		measurement.SetMetric("LowPrice", data.Metric[float64]{
			Raw: lowPrice, Exact: decimal.NewFromFloat64(lowPrice),
		})
		measurement.SetMetric("HighPrice", data.Metric[float64]{
			Raw: highPrice, Exact: decimal.NewFromFloat64(highPrice),
		})
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
			So(training.engine.Len(), ShouldBeGreaterThan, 0)

			census := training.engine.Census()
			So(census[string("enter")], ShouldBeGreaterThan, 0)
			So(census[string("exit")], ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given a past run with a stored excursion that does not clear friction", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := trainingFixture(t, ctx, detection(60000, 60010))

		Convey("It remains in WAITING without training on the losing excursion", func() {
			So(training.Status(), ShouldEqual, runtime.WAITING)
			So(training.engine.Len(), ShouldEqual, 0)
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

				trade := data.NewMeasurement("spot:trade", map[string]data.Metric[float64]{
					"price": {Raw: exact.Float64(), Exact: exact},
				})
				trade.Epoch = epoch
				trade.Label = "BTC/USD"
				trade.Tick = tick
				trade.SeqIdx = tick
				writer.Add("measurements", data.Publication{Measurement: trade})
			}
		})

		Convey("It detects the excursion and learns from it in the same pass", func() {
			So(training.Status(), ShouldEqual, runtime.READY)
			census := training.engine.Census()
			So(census[string("enter")], ShouldBeGreaterThan, 0)
			So(census[string("exit")], ShouldBeGreaterThan, 0)
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
