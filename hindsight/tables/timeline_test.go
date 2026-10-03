package tables_test

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/tests/tablestest"
)

func TestCatalog_Timeline(t *testing.T) {
	Convey("Given an Iceberg catalog with measurements", t, func() {
		catalog := tablestest.New(t)
		ctx := context.Background()
		at := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
		epoch := int64(100)

		err := catalog.RecordRun(ctx, tables.Run{
			Epoch:     epoch,
			StartedAt: at,
			BuildID:   "test",
			Status:    "ACTIVE",
		})
		So(err, ShouldBeNil)

		writer := tables.NewWriter(catalog, epoch)

		sig1 := data.NewMeasurement[float64]("cvd", nil)
		sig1.Label = "BTC/USD"
		sig1.SeqIdx = 10
		sig1.At = at
		sig1.Maturity = 1.0
		sig1.SetMetric("delta", data.NewMetric[float64](
			"delta",
			data.UnitDimensionless,
			data.TimescaleInstantaneous,
			100.0,
			100.0,
		).Write(100.0))
		writer.Add("measurements", data.Publication{Measurement: sig1})

		sig2 := data.NewMeasurement[float64]("cvd", nil)
		sig2.Label = "BTC/USD"
		sig2.SeqIdx = 20
		sig2.At = at.Add(time.Second)
		sig2.Maturity = 1.0
		sig2.SetMetric("delta", data.NewMetric[float64](
			"delta",
			data.UnitDimensionless,
			data.TimescaleInstantaneous,
			150.0,
			150.0,
		).Write(150.0))
		writer.Add("measurements", data.Publication{Measurement: sig2})

		So(writer.CommitReady(ctx, true), ShouldBeNil)

		Convey("Timeline retrieves the stream of measurements ordered by seqIdx", func() {
			var reconstructed []*data.Measurement[float64]

			for reading := range catalog.Timeline(ctx, epoch, "BTC/USD", 0, 0) {
				reconstructed = append(reconstructed, reading)
			}

			So(len(reconstructed), ShouldEqual, 2)
			So(reconstructed[0].SeqIdx, ShouldEqual, 10)
			So(reconstructed[1].SeqIdx, ShouldEqual, 20)
		})

		Convey("Labels lists all distinct labels in the epoch", func() {
			labels, err := catalog.Labels(ctx, epoch)
			So(err, ShouldBeNil)
			So(labels, ShouldResemble, []string{"BTC/USD"})
		})
	})
}

func TestCatalog_Trades(t *testing.T) {
	Convey("Given an Iceberg catalog with mixed measurements", t, func() {
		catalog := tablestest.New(t)
		ctx := context.Background()
		epoch1 := int64(100)
		epoch2 := int64(200)

		writer1 := tables.NewWriter(catalog, epoch1)

		trade1 := data.NewMeasurement[float64]("spot:trade", nil)
		trade1.Label = "BTC/USD"
		trade1.Epoch = epoch1
		trade1.Tick = 2
		trade1.SeqIdx = 20
		writer1.Add("measurements", data.Publication{Measurement: trade1})

		trade2 := data.NewMeasurement[float64]("spot:trade", nil)
		trade2.Label = "BTC/USD"
		trade2.Epoch = epoch1
		trade2.Tick = 1
		trade2.SeqIdx = 10
		writer1.Add("measurements", data.Publication{Measurement: trade2})

		tradeETH := data.NewMeasurement[float64]("spot:trade", nil)
		tradeETH.Label = "ETH/USD"
		tradeETH.Epoch = epoch1
		tradeETH.Tick = 1
		tradeETH.SeqIdx = 5
		writer1.Add("measurements", data.Publication{Measurement: tradeETH})

		nonTrade := data.NewMeasurement[float64]("spot:level3", nil)
		nonTrade.Label = "BTC/USD"
		nonTrade.Epoch = epoch1
		nonTrade.Tick = 0
		nonTrade.SeqIdx = 1
		writer1.Add("measurements", data.Publication{Measurement: nonTrade})

		So(writer1.CommitReady(ctx, true), ShouldBeNil)

		writer2 := tables.NewWriter(catalog, epoch2)
		trade3 := data.NewMeasurement[float64]("spot:trade", nil)
		trade3.Label = "BTC/USD"
		trade3.Epoch = epoch2
		trade3.Tick = 1
		trade3.SeqIdx = 30
		writer2.Add("measurements", data.Publication{Measurement: trade3})

		So(writer2.CommitReady(ctx, true), ShouldBeNil)

		Convey("Trades retrieves only spot:trade, groups by label, and sorts by epoch and tick", func() {
			var trades []*data.Measurement[float64]

			for trade := range catalog.Trades(ctx) {
				trades = append(trades, trade)
			}

			So(len(trades), ShouldEqual, 4)

			So(trades[0].Label, ShouldEqual, "BTC/USD")
			So(trades[0].Epoch, ShouldEqual, epoch1)
			So(trades[0].Tick, ShouldEqual, 1)

			So(trades[1].Label, ShouldEqual, "BTC/USD")
			So(trades[1].Epoch, ShouldEqual, epoch1)
			So(trades[1].Tick, ShouldEqual, 2)

			So(trades[2].Label, ShouldEqual, "BTC/USD")
			So(trades[2].Epoch, ShouldEqual, epoch2)
			So(trades[2].Tick, ShouldEqual, 1)

			So(trades[3].Label, ShouldEqual, "ETH/USD")
			So(trades[3].Epoch, ShouldEqual, epoch1)
			So(trades[3].Tick, ShouldEqual, 1)
		})

		Convey("Trades for a specific epoch retrieves only that epoch", func() {
			var trades []*data.Measurement[float64]

			for trade := range catalog.Trades(ctx, epoch1) {
				trades = append(trades, trade)
			}

			So(len(trades), ShouldEqual, 3)
		})
	})
}

func TestCatalog_Detections(t *testing.T) {
	Convey("Given an Iceberg catalog with detector measurements", t, func() {
		catalog := tablestest.New(t)
		ctx := context.Background()
		epoch1 := int64(100)
		epoch2 := int64(200)

		writer1 := tables.NewWriter(catalog, epoch1)

		det1 := data.NewMeasurement[float64]("detector", nil)
		det1.Label = "BTC/USD"
		det1.Epoch = epoch1
		det1.Tick = 20
		det1.SeqIdx = 200
		det1.SetMetric("LowTick", data.Metric[float64]{Raw: 10})
		det1.SetMetric("HighTick", data.Metric[float64]{Raw: 20})
		det1.SetMetric("LowSeqIdx", data.Metric[float64]{Raw: 100})
		det1.SetMetric("HighSeqIdx", data.Metric[float64]{Raw: 200})
		writer1.Add("measurements", data.Publication{Measurement: det1})

		det2 := data.NewMeasurement[float64]("detector", nil)
		det2.Label = "BTC/USD"
		det2.Epoch = epoch1
		det2.Tick = 10
		det2.SeqIdx = 100
		det2.SetMetric("LowTick", data.Metric[float64]{Raw: 5})
		det2.SetMetric("HighTick", data.Metric[float64]{Raw: 10})
		writer1.Add("measurements", data.Publication{Measurement: det2})

		detETH := data.NewMeasurement[float64]("detector", nil)
		detETH.Label = "ETH/USD"
		detETH.Epoch = epoch1
		detETH.Tick = 15
		detETH.SeqIdx = 150
		detETH.SetMetric("LowTick", data.Metric[float64]{Raw: 10})
		detETH.SetMetric("HighTick", data.Metric[float64]{Raw: 15})
		writer1.Add("measurements", data.Publication{Measurement: detETH})

		nonDet := data.NewMeasurement[float64]("spot:trade", nil)
		nonDet.Label = "BTC/USD"
		nonDet.Epoch = epoch1
		nonDet.Tick = 10
		nonDet.SeqIdx = 50
		writer1.Add("measurements", data.Publication{Measurement: nonDet})

		So(writer1.CommitReady(ctx, true), ShouldBeNil)

		writer2 := tables.NewWriter(catalog, epoch2)

		det3 := data.NewMeasurement[float64]("detector", nil)
		det3.Label = "BTC/USD"
		det3.Epoch = epoch2
		det3.Tick = 10
		det3.SeqIdx = 300
		det3.SetMetric("LowTick", data.Metric[float64]{Raw: 1})
		det3.SetMetric("HighTick", data.Metric[float64]{Raw: 10})
		writer2.Add("measurements", data.Publication{Measurement: det3})

		So(writer2.CommitReady(ctx, true), ShouldBeNil)

		Convey("Detections retrieves only source=detector, groups by label, and sorts by epoch and tick", func() {
			var detections []*data.Measurement[float64]

			for det := range catalog.Detections(ctx) {
				detections = append(detections, det)
			}

			So(len(detections), ShouldEqual, 4)

			So(detections[0].Label, ShouldEqual, "BTC/USD")
			So(detections[0].Epoch, ShouldEqual, epoch1)
			So(detections[0].Tick, ShouldEqual, 10)

			So(detections[1].Label, ShouldEqual, "BTC/USD")
			So(detections[1].Epoch, ShouldEqual, epoch1)
			So(detections[1].Tick, ShouldEqual, 20)

			So(detections[2].Label, ShouldEqual, "BTC/USD")
			So(detections[2].Epoch, ShouldEqual, epoch2)
			So(detections[2].Tick, ShouldEqual, 10)

			So(detections[3].Label, ShouldEqual, "ETH/USD")
			So(detections[3].Epoch, ShouldEqual, epoch1)
			So(detections[3].Tick, ShouldEqual, 15)
		})

		Convey("Excursions alias returns identical results for a specific epoch", func() {
			var excursions []*data.Measurement[float64]

			for exc := range catalog.Excursions(ctx, epoch1) {
				excursions = append(excursions, exc)
			}

			So(len(excursions), ShouldEqual, 3)
		})

		Convey("DetectionTicks correctly extracts lowTick and highTick", func() {
			lowTick, highTick, err := tables.DetectionTicks(det1)
			So(err, ShouldBeNil)
			So(lowTick, ShouldEqual, 10)
			So(highTick, ShouldEqual, 20)
		})

		Convey("DetectionTicks validates invalid inputs", func() {
			_, _, err := tables.DetectionTicks(nil)
			So(err, ShouldNotBeNil)

			emptyDet := data.NewMeasurement[float64]("detector", nil)
			_, _, err = tables.DetectionTicks(emptyDet)
			So(err, ShouldNotBeNil)
		})
	})
}

func TestCatalog_SignalLogic(t *testing.T) {
	Convey("Given an Iceberg catalog with signal and logic measurements across ticks", t, func() {
		catalog := tablestest.New(t)
		ctx := context.Background()
		targetEpoch := int64(500)
		otherEpoch := int64(600)

		writer := tables.NewWriter(catalog, targetEpoch)

		addMeasurement := func(source string, symbol string, epoch int64, tick int64, seqIdx int64) {
			measurement := data.NewMeasurement[float64](source, nil)
			measurement.Label = symbol
			measurement.Epoch = epoch
			measurement.Tick = tick
			measurement.SeqIdx = seqIdx
			writer.Add("measurements", data.Publication{Measurement: measurement})
		}

		// Within window: ticks 10 to 15, BTC/USD, epoch 500
		addMeasurement("cvd", "BTC/USD", targetEpoch, 10, 101)
		addMeasurement("hawkes", "BTC/USD", targetEpoch, 10, 102)
		addMeasurement("resonance", "BTC/USD", targetEpoch, 11, 103)
		addMeasurement("liquidity", "BTC/USD", targetEpoch, 12, 104)
		addMeasurement("manifold", "BTC/USD", targetEpoch, 15, 105)

		// Outside window: tick before 10 and tick after 15
		addMeasurement("cvd", "BTC/USD", targetEpoch, 5, 90)
		addMeasurement("cvd", "BTC/USD", targetEpoch, 20, 120)

		// Outside source: spot:trade and detector at tick 10
		addMeasurement("spot:trade", "BTC/USD", targetEpoch, 10, 100)
		addMeasurement("detector", "BTC/USD", targetEpoch, 10, 106)

		// Outside symbol: ETH/USD at tick 10
		addMeasurement("cvd", "ETH/USD", targetEpoch, 10, 107)

		So(writer.CommitReady(ctx, true), ShouldBeNil)

		// Outside epoch: epoch 600 at tick 10
		writerOther := tables.NewWriter(catalog, otherEpoch)
		otherMeas := data.NewMeasurement[float64]("cvd", nil)
		otherMeas.Label = "BTC/USD"
		otherMeas.Epoch = otherEpoch
		otherMeas.Tick = 10
		otherMeas.SeqIdx = 300
		writerOther.Add("measurements", data.Publication{Measurement: otherMeas})
		So(writerOther.CommitReady(ctx, true), ShouldBeNil)

		Convey("SignalLogic retrieves exactly signal and logic measurements in tick window", func() {
			var results []*data.Measurement[float64]

			for measurement := range catalog.SignalLogic(ctx, targetEpoch, "BTC/USD", 10, 15) {
				results = append(results, measurement)
			}

			So(len(results), ShouldEqual, 5)

			So(results[0].Source, ShouldEqual, "cvd")
			So(results[0].Tick, ShouldEqual, 10)

			So(results[1].Source, ShouldEqual, "hawkes")
			So(results[1].Tick, ShouldEqual, 10)

			So(results[2].Source, ShouldEqual, "resonance")
			So(results[2].Tick, ShouldEqual, 11)

			So(results[3].Source, ShouldEqual, "liquidity")
			So(results[3].Tick, ShouldEqual, 12)

			So(results[4].Source, ShouldEqual, "manifold")
			So(results[4].Tick, ShouldEqual, 15)
		})

		Convey("DetectionSignalLogic retrieves matching window from detection measurement", func() {
			det := data.NewMeasurement[float64]("detector", nil)
			det.Epoch = targetEpoch
			det.Label = "BTC/USD"
			det.Tick = 15
			det.SetMetric("LowTick", data.Metric[float64]{Raw: 10})
			det.SetMetric("HighTick", data.Metric[float64]{Raw: 15})

			var results []*data.Measurement[float64]

			for measurement := range catalog.DetectionSignalLogic(ctx, det) {
				results = append(results, measurement)
			}

			So(len(results), ShouldEqual, 5)
			So(results[0].Tick, ShouldEqual, 10)
			So(results[4].Tick, ShouldEqual, 15)
		})

		Convey("SignalLogic with source overrides filters specifically", func() {
			var results []*data.Measurement[float64]

			for measurement := range catalog.SignalLogic(ctx, targetEpoch, "BTC/USD", 10, 15, "resonance") {
				results = append(results, measurement)
			}

			So(len(results), ShouldEqual, 1)
			So(results[0].Source, ShouldEqual, "resonance")
			So(results[0].Tick, ShouldEqual, 11)
		})
	})
}
