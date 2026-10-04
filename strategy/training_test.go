package strategy

import (
	"context"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/tests/tablestest"
)

func TestTraining_Train(t *testing.T) {
	Convey("Given a Training component with a stored excursion in Iceberg", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		catalog := tablestest.New(t)
		epoch := int64(100)

		err := catalog.RecordRun(ctx, tables.Run{
			Epoch:     epoch,
			StartedAt: time.Now().UTC(),
			Status:    "ACTIVE",
		})
		So(err, ShouldBeNil)

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

		detection := data.NewMeasurement[float64]("detector", nil)
		detection.Epoch = epoch
		detection.Label = "BTC/USD"
		detection.Tick = 15
		detection.SeqIdx = 150
		detection.SetMetric("LowTick", data.Metric[float64]{Raw: 10})
		detection.SetMetric("HighTick", data.Metric[float64]{Raw: 15})
		detection.SetMetric("LowPrice", data.Metric[float64]{
			Raw: 60000.0, Exact: decimal.NewFromFloat64(60000.0),
		})
		detection.SetMetric("HighPrice", data.Metric[float64]{
			Raw: 63000.0, Exact: decimal.NewFromFloat64(63000.0),
		})
		writer.Add("measurements", data.Publication{Measurement: detection})

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

		desk := broker.NewDesk(ctx, nil, price)
		training := NewTraining(ctx, data.NewArenaOwner(4096), price, desk, catalog, nil)
		training.Transition(runtime.WAITING)
		training.Train()

		deadline := time.After(2 * time.Second)

		for training.Status() != runtime.READY {
			select {
			case <-deadline:
				t.Fatal("timed out waiting for the trie to load")
			case <-time.After(20 * time.Millisecond):
			}
		}

		Convey("It loads enter and exit associations into the trie and opens trading", func() {
			So(training.engine.Len(), ShouldBeGreaterThan, 0)

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
