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
	"github.com/theapemachine/symm/ui"
)

func TestTraining(t *testing.T) {
	Convey("Training system lifecycle and execution", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		arena := data.NewArenaOwner(4096)
		price := broker.NewPrice(ctx, nil, nil, nil, nil)
		price.SetFee("BTC/USD", kraken.TradeVolumeFee{
			Fee: decimal.NewFromFloat64(0.26),
		})
		price.SetReferenceCash(decimal.NewFromFloat64(10000))

		uiTee := ui.NewUITee(ctx, "ui-test", 1, func(_ *data.Measurement[float64]) bool { return true })
		uiTee.Transition(runtime.READY)

		catalog := tablestest.New(t)
		training := NewTraining(ctx, arena, price, nil, catalog, uiTee)

		Convey("Initial state is INIT and cognition tree is queryable", func() {
			So(training.Status(), ShouldEqual, runtime.INIT)
			So(training.Source(), ShouldEqual, "training")
			So(training.Arena(), ShouldNotBeNil)

			treeExport := training.CognitionTree()
			So(treeExport.Root, ShouldNotBeNil)
		})

		Convey("Step during INIT develops grid and transitions to WAITING upon settlement", func() {
			for sequence := int64(1); sequence <= 10; sequence++ {
				measurement := arena.NewMeasurement("spot:ticker")
				measurement.Label = "BTC/USD"
				measurement.Epoch = 100
				measurement.SeqIdx = sequence
				measurement.At = time.Unix(sequence, 0)
				measurement.SetMetric("price", data.Metric[float64]{Raw: 60000.0 + float64(sequence)})

				out := training.Step(measurement)
				So(out, ShouldNotBeNil)
			}

			// Force settlement of grid to verify WAITING transition
			training.grid.Settle()
			nextMeasurement := arena.NewMeasurement("spot:ticker")
			nextMeasurement.Label = "BTC/USD"
			nextMeasurement.Epoch = 100
			nextMeasurement.SeqIdx = 11
			nextMeasurement.At = time.Unix(11, 0)
			nextMeasurement.SetMetric("price", data.Metric[float64]{Raw: 60020.0})

			out := training.Step(nextMeasurement)
			So(out, ShouldNotBeNil)
			So(training.Status(), ShouldEqual, runtime.WAITING)
		})

		Convey("Historical tape in catalog trains evaluator and updates skill", func() {
			epoch := int64(200)

			err := catalog.RecordRun(ctx, tables.Run{
				Epoch:     epoch,
				StartedAt: time.Now(),
				Status:    "COMPLETED",
			})
			So(err, ShouldBeNil)

			writer := tables.NewWriter(catalog, epoch)

			// Generate tape: precursor ticks around 100, drop to low 95, rise to high 120, pullback to 114 (>= 20% of 25 move)
			prices := []float64{
				100, 100, 100, 99, 98, 97, 96, 95.5, 95, 95,
				97, 100, 105, 110, 115, 118, 120,
				117, 115, 114,
			}

			for sequence, priceValue := range prices {
				seqIdx := int64(sequence + 1)
				measurement := data.NewMeasurement("spot:ticker", map[string]data.Metric[float64]{
					"ask": {
						Raw:   priceValue + 0.1,
						Exact: decimal.NewFromFloat64(priceValue + 0.1),
					},
					"bid": {
						Raw:   priceValue - 0.1,
						Exact: decimal.NewFromFloat64(priceValue - 0.1),
					},
					"price": {
						Raw:   priceValue,
						Exact: decimal.NewFromFloat64(priceValue),
					},
				})
				measurement.Epoch = epoch
				measurement.Label = "BTC/USD"
				measurement.SeqIdx = seqIdx
				measurement.At = time.Unix(seqIdx, 0)

				writer.Add(tables.Measurements, data.NewPublication(measurement, nil))

				signal := data.NewMeasurement("cvd", map[string]data.Metric[float64]{
					"cumulative_volume_delta": {Raw: float64(sequence) * 5.0},
				})
				signal.Epoch = epoch
				signal.Label = "BTC/USD"
				signal.SeqIdx = seqIdx
				signal.At = time.Unix(seqIdx, 0)
				writer.Add(tables.Measurements, data.NewPublication(signal, nil))
			}

			err = writer.CommitReady(ctx, true)
			So(err, ShouldBeNil)

			maxSeq, hasEdge := training.processRun(epoch, 0)
			So(maxSeq, ShouldEqual, int64(len(prices)))

			// Check that detector extracted the excursion and skill recorded historical outcomes
			So(training.skill.HistOpportunities(), ShouldBeGreaterThanOrEqualTo, 1)
			So(training.skill.HistCorrectEnter(), ShouldBeGreaterThanOrEqualTo, 1)
			So(training.skill.HistMeanReturn(), ShouldBeGreaterThan, 0)

			_ = hasEdge
		})
	})
}
