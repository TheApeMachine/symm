package strategy

import (
	"context"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/tests/tablestest"
)

func TestTraining_Train(t *testing.T) {
	Convey("Given a Training component with stored excursions in Iceberg", t, func() {
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

		price := broker.NewPrice(ctx, nil, nil, nil, nil)
		price.SetFee("BTC/USD", kraken.TradeVolumeFee{
			Fee: decimal.NewFromFloat64(0.26),
		})
		price.SetReferenceCash(decimal.NewFromFloat64(10000))

		writer := tables.NewWriter(catalog, epoch)

		// 1. Write a detector measurement representing an excursion
		det := data.NewMeasurement[float64]("detector", nil)
		det.Epoch = epoch
		det.Label = "BTC/USD"
		det.Tick = 15
		det.SeqIdx = 150

		lowestPrice := decimal.NewFromFloat64(60000.0)
		highestPrice := decimal.NewFromFloat64(63000.0)

		det.SetMetric("LowTick", data.Metric[float64]{Raw: 10})
		det.SetMetric("HighTick", data.Metric[float64]{Raw: 15})
		det.SetMetric("LowSeqIdx", data.Metric[float64]{Raw: 100})
		det.SetMetric("HighSeqIdx", data.Metric[float64]{Raw: 150})

		lowPriceMetric := data.Metric[float64]{Raw: 60000.0, Exact: lowestPrice}
		highPriceMetric := data.Metric[float64]{Raw: 63000.0, Exact: highestPrice}
		det.SetMetric("LowPrice", lowPriceMetric)
		det.SetMetric("HighPrice", highPriceMetric)

		writer.Add("measurements", data.Publication{Measurement: det})

		// 2. Write signal and logic measurements within tick window [10, 15]
		addMeas := func(source string, tick int64, seqIdx int64) {
			meas := data.NewMeasurement[float64](source, nil)
			meas.Epoch = epoch
			meas.Label = "BTC/USD"
			meas.Tick = tick
			meas.SeqIdx = seqIdx
			meas.Maturity = 1.0
			meas.SNR = 2.0
			meas.SNRDefined = true
			meas.SetMetric("value", data.Metric[float64]{
				Label:  "value",
				Raw:    1.5,
				Center: 0,
				Scale:  1,
				Region: 1,
			})
			writer.Add("measurements", data.Publication{Measurement: meas})
		}

		addMeas("cvd", 10, 101)
		addMeas("hawkes", 10, 102)
		addMeas("resonance", 11, 103)
		addMeas("liquidity", 12, 104)
		addMeas("manifold", 15, 105)

		So(writer.CommitReady(ctx, true), ShouldBeNil)

		arena := data.NewArenaOwner(4096)
		training := NewTraining(ctx, arena, price, catalog, nil, nil)
		training.Transition(runtime.WAITING)

		training.Train()

		// Wait for historical training to process the detection
		timeout := time.After(2 * time.Second)
		processed := false

		for !processed {
			select {
			case <-timeout:
				t.Fatal("timed out waiting for historical training to process excursion")
			case <-time.After(20 * time.Millisecond):
				if training.skill.HistOpportunities() > 0 {
					processed = true
				}
			}
		}

		Convey("Historical training trains trie and records opportunity", func() {
			So(training.skill.HistOpportunities(), ShouldBeGreaterThanOrEqualTo, 1)

			upFragments, _, _, _, _ := training.skill.Fragments()
			So(upFragments, ShouldBeGreaterThanOrEqualTo, 1)

			So(training.engine.Len(), ShouldBeGreaterThan, 0)
		})
	})
}
