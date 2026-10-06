package strategy

import (
	"context"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/tests/tablestest"
)

/*
storedTrade writes one spot:trade row with an explicit sequence index, tick,
and timestamp, so a test can make sequence order disagree with tick order or
leave the timestamp unset.
*/
func storedTrade(seqIdx, tick int64, price string, at time.Time) func(*tables.Writer, int64) {
	return func(writer *tables.Writer, epoch int64) {
		exact, err := decimal.NewFromString(price)
		So(err, ShouldBeNil)

		trade := data.NewMeasurement(epoch, "BTC/USD", "spot:trade", seqIdx, tick)
		trade.At = at
		trade.From = at
		trade.Write(data.NewExactMetric("price", exact, data.UnitPrice, data.TimescaleTick))
		writer.Add("measurements", trade)
	}
}

/*
pricedRow writes one non-trade row that carries a price metric, the way a
reporter snapshot or a signal can.
*/
func pricedRow(source string, tick int64, price float64) func(*tables.Writer, int64) {
	return func(writer *tables.Writer, epoch int64) {
		measurement := data.NewMeasurement(epoch, "BTC/USD", source, tick, tick)
		measurement.At = time.Now().UTC()
		measurement.From = measurement.At
		measurement.Write(data.NewExactMetric(
			"price", decimal.NewFromFloat64(price), data.UnitPrice, data.TimescaleTick,
		))
		writer.Add("measurements", measurement)
	}
}

/*
tapeWindow returns the training's first stored detection with its start and
C ticks.
*/
func tapeWindow(ctx context.Context, training *Training) (*data.Measurement, int64, int64) {
	detectionRow := firstDetection(ctx, training)
	So(detectionRow, ShouldNotBeNil)

	startTick, _, cTick, err := tables.DetectionTicks(detectionRow)
	So(err, ShouldBeNil)

	return detectionRow, startTick, cTick
}

func TestChart_PriceTape(t *testing.T) {
	Convey("Given a stored excursion over its trade tape whose data files then fail to read", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		Convey("priceTape returns the read failure instead of an empty or fabricated tape", func() {
			training := trainingSetup(t, ctx, regimeTape, detection(excursionUp, 60000, 63000))
			detectionRow := firstDetection(ctx, training)
			So(detectionRow, ShouldNotBeNil)

			startTick, _, cTick, err := tables.DetectionTicks(detectionRow)
			So(err, ShouldBeNil)

			// Healthy storage reads the stored trades, so the failure below
			// is the read itself, not the shape of the window.
			points, err := training.Rehearsal.Chart.priceTape(ctx, detectionRow, startTick, cTick)
			So(err, ShouldBeNil)
			So(points, ShouldNotBeEmpty)

			tablestest.DropDataFiles(t, training.catalog, tables.Measurements)

			points, err = training.Rehearsal.Chart.priceTape(ctx, detectionRow, startTick, cTick)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "unable to read price tape")
			So(err.Error(), ShouldContainSubstring, "[iceberg]")
			So(points, ShouldBeNil)
		})
	})

	Convey("Given a stored detection whose window holds no spot:trade row", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		bare := detectionRowAt(excursionUp, 7, 10, 15, 60000, 63000)

		Convey("priceTape errors NotFound naming the window instead of drawing a B→C line", func() {
			training := trainingSetup(t, ctx, regimeTape, bare)
			detectionRow, startTick, cTick := tapeWindow(ctx, training)

			points, err := training.Rehearsal.Chart.priceTape(ctx, detectionRow, startTick, cTick)
			So(err, ShouldNotBeNil)
			So(errnie.IsNotFound(err), ShouldBeTrue)
			So(err.Error(), ShouldContainSubstring, "no spot:trade price tape")
			So(err.Error(), ShouldContainSubstring, "BTC/USD epoch 100 ticks 7..15")
			So(points, ShouldBeNil)
		})

		Convey("learn returns that error and stores no fragment", func() {
			training := trainingSetup(t, ctx, regimeTape, bare)
			detectionRow, _, _ := tapeWindow(ctx, training)

			_, _, _, err := training.Rehearsal.learn(ctx, detectionRow, 0)
			So(err, ShouldNotBeNil)
			So(errnie.IsNotFound(err), ShouldBeTrue)
			So(err.Error(), ShouldContainSubstring, "no spot:trade price tape")
			So(training.Rehearsal.Chart.Fragments(), ShouldBeEmpty)
		})

		Convey("The training pass halts Internal", func() {
			training := trainingFixture(t, ctx, regimeTape, bare)

			haltedInternal(training)
			So(training.Error().Error(), ShouldContainSubstring, "failed during rehearsal")
			So(training.Error().Error(), ShouldContainSubstring, "no spot:trade price tape")
			So(training.Rehearsal.Chart.Fragments(), ShouldBeEmpty)
		})
	})

	Convey("Given a window whose only priced rows come from other sources", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		training := trainingSetup(
			t, ctx, regimeTape, detectionRowAt(excursionUp, 7, 10, 15, 60000, 63000),
			pricedRow("training", 11, 61000),
			pricedRow("cvd", 12, 61500),
		)
		detectionRow, startTick, cTick := tapeWindow(ctx, training)

		Convey("priceTape ignores them and errors on the missing trade tape", func() {
			points, err := training.Rehearsal.Chart.priceTape(ctx, detectionRow, startTick, cTick)
			So(err, ShouldNotBeNil)
			So(errnie.IsNotFound(err), ShouldBeTrue)
			So(points, ShouldBeNil)
		})
	})

	Convey("Given trades whose sequence order disagrees with their tick order", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		now := time.Now().UTC()

		training := trainingSetup(
			t, ctx, regimeTape, detectionRowAt(excursionUp, 7, 10, 15, 60000, 63000),
			// Sequence indices run backwards against the ticks, so the
			// SeqIdx-ordered Timeline yields ticks 15, 14, 12, 9, 7.
			storedTrade(50, 7, "60200", now),
			storedTrade(40, 9, "60100", now),
			storedTrade(30, 12, "61500", now),
			storedTrade(20, 14, "62500", now),
			storedTrade(10, 15, "63000", now),
			// Inside the window only by sequence index: Timeline admits
			// it, the fragment's tick window must not.
			storedTrade(12, 40, "99999", now),
			// A priced non-trade row at a tick no trade has.
			pricedRow("training", 11, 1),
		)
		detectionRow, startTick, cTick := tapeWindow(ctx, training)

		points, err := training.Rehearsal.Chart.priceTape(ctx, detectionRow, startTick, cTick)
		So(err, ShouldBeNil)

		Convey("priceTape keeps only the window's trades, in tick order, with X as the index", func() {
			So(points, ShouldHaveLength, 5)

			wantTicks := []int64{7, 9, 12, 14, 15}
			wantPrices := []float64{60200, 60100, 61500, 62500, 63000}

			for index, point := range points {
				So(point.X, ShouldEqual, index)
				So(point.Tick, ShouldEqual, wantTicks[index])
				So(point.Y, ShouldEqual, wantPrices[index])
				So(point.Time, ShouldEqual, now.UnixMilli())
			}
		})

		Convey("pointAt lands a frame on the first trade at or after its tick", func() {
			ticks := []int64{7, 8, 10, 13, 15, 16}

			So(pointAt(points, ticks, 0), ShouldEqual, 0) // tick 7  -> trade 7
			So(pointAt(points, ticks, 1), ShouldEqual, 1) // tick 8  -> trade 9
			So(pointAt(points, ticks, 2), ShouldEqual, 2) // tick 10 -> trade 12
			So(pointAt(points, ticks, 3), ShouldEqual, 3) // tick 13 -> trade 14
			So(pointAt(points, ticks, 4), ShouldEqual, 4) // tick 15 -> trade 15
			So(pointAt(points, ticks, 5), ShouldEqual, -1)
			So(pointAt(points, ticks, -1), ShouldEqual, -1)
		})
	})

	Convey("Given a window with a trade that has no timestamp", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		now := time.Now().UTC()

		training := trainingSetup(
			t, ctx, regimeTape, detectionRowAt(excursionUp, 7, 10, 15, 60000, 63000),
			storedTrade(7, 7, "60200", now),
			storedTrade(12, 12, "61500", time.Time{}),
			storedTrade(15, 15, "63000", now),
		)
		detectionRow, startTick, cTick := tapeWindow(ctx, training)

		Convey("priceTape errors on that row instead of stamping it with the current time", func() {
			// The stored row itself is invalid, so the tape read halts on it
			// before the chart ever sees a zero timestamp.
			points, err := training.Rehearsal.Chart.priceTape(ctx, detectionRow, startTick, cTick)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "source spot:trade")
			So(err.Error(), ShouldContainSubstring, "tick 12")
			So(err.Error(), ShouldContainSubstring, "at is required")
			So(points, ShouldBeNil)
		})
	})
}
