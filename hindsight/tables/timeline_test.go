package tables_test

import (
	"context"
	"iter"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/tests/tablestest"
)

/*
drainSeq consumes a measurement sequence and returns its rows and the first
error it yielded.
*/
func drainSeq(seq iter.Seq2[*data.Measurement, error]) ([]*data.Measurement, error) {
	var rows []*data.Measurement

	for measurement, err := range seq {
		if err != nil {
			return rows, err
		}

		rows = append(rows, measurement)
	}

	return rows, nil
}

func TestCatalog_TimelineReadFailure(t *testing.T) {
	Convey("Given a catalog holding trade and signal rows, and detector rows in the detections table, for one run", t, func() {
		ctx := context.Background()
		catalog := tablestest.New(t)
		epoch := int64(100)
		writer := tables.NewWriter(catalog, epoch)
		detections := make([]*data.Measurement, 0, 4)

		for tick := int64(1); tick <= 4; tick++ {
			for _, source := range []string{"spot:trade", "detector", "cvd"} {
				measurement := data.NewMeasurement(epoch, "BTC/USD", source, tick, tick)
				measurement.At = time.Now().UTC()
				measurement.From = measurement.At
				measurement.Write(data.NewMetric("value", float64(tick), data.UnitCount, data.TimescaleTick))

				if source == "detector" {
					detections = append(detections, measurement)
					continue
				}

				writer.Add(tables.Measurements, measurement)
			}
		}

		So(writer.CommitReady(ctx, true), ShouldBeNil)
		So(catalog.Append(ctx, tables.Detections, epoch, detections), ShouldBeNil)

		reads := map[string]func() iter.Seq2[*data.Measurement, error]{
			"Trades":      func() iter.Seq2[*data.Measurement, error] { return catalog.Trades(ctx, epoch) },
			"Detections":  func() iter.Seq2[*data.Measurement, error] { return catalog.Detections(ctx, epoch) },
			"SignalLogic": func() iter.Seq2[*data.Measurement, error] { return catalog.SignalLogic(ctx, epoch, "BTC/USD", 0, 0) },
		}

		// Scan and Timeline return every measurements-table source for the
		// run/label (trade and cvd), so a healthy read holds 8 rows.
		displayReads := map[string]func() iter.Seq2[*data.Measurement, error]{
			"Scan": func() iter.Seq2[*data.Measurement, error] {
				return catalog.Scan(ctx, tables.Measurements, epoch, nil, 0)
			},
			"Timeline": func() iter.Seq2[*data.Measurement, error] { return catalog.Timeline(ctx, epoch, "BTC/USD", 1, 4) },
		}

		Convey("Every read yields its rows while storage is healthy", func() {
			for name, read := range reads {
				rows, err := drainSeq(read())
				So(name+": "+errString(err), ShouldEqual, name+": <nil>")
				So(len(rows), ShouldEqual, 4)
			}

			for name, read := range displayReads {
				rows, err := drainSeq(read())
				So(name+": "+errString(err), ShouldEqual, name+": <nil>")
				So(len(rows), ShouldEqual, 8)
			}

			labels, err := catalog.Labels(ctx, epoch)
			So(err, ShouldBeNil)
			So(labels, ShouldResemble, []string{"BTC/USD"})
		})

		Convey("When the table's data files fail to read", func() {
			tablestest.DropDataFiles(t, catalog, tables.Measurements)
			tablestest.DropDataFiles(t, catalog, tables.Detections)

			Convey("Every read yields the storage error instead of ending as an empty stream", func() {
				for name, read := range reads {
					rows, err := drainSeq(read())
					So(name+": "+errString(err), ShouldContainSubstring, "[iceberg]")
					So(rows, ShouldBeEmpty)
				}

				for name, read := range displayReads {
					rows, err := drainSeq(read())
					So(name+": "+errString(err), ShouldContainSubstring, "[iceberg]")
					So(rows, ShouldBeEmpty)
				}

				labels, err := catalog.Labels(ctx, epoch)
				So(err, ShouldNotBeNil)
				So(labels, ShouldBeNil)
			})
		})

		Convey("When the arguments are invalid", func() {
			rows, err := drainSeq(catalog.SignalLogic(ctx, 0, "BTC/USD", 0, 0))

			Convey("The validation error is yielded, not swallowed as no rows", func() {
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "positive epoch")
				So(rows, ShouldBeEmpty)
			})
		})
	})
}

func errString(err error) string {
	if err == nil {
		return "<nil>"
	}

	return err.Error()
}

func TestCatalog_TimelineTickWindow(t *testing.T) {
	Convey("Given rows whose sequence index and tick live in different ranges", t, func() {
		ctx := context.Background()
		catalog := tablestest.New(t)
		epoch := int64(101)
		writer := tables.NewWriter(catalog, epoch)

		// seqIdx 1..6, tick 100..105: a seqIdx-based window [1, 4] would
		// match every row, a tick window [1, 4] matches none.
		for seq := int64(1); seq <= 6; seq++ {
			measurement := data.NewMeasurement(epoch, "BTC/USD", "spot:trade", seq, 99+seq)
			measurement.At = time.Now().UTC()
			measurement.From = measurement.At
			measurement.Write(data.NewMetric("value", float64(seq), data.UnitCount, data.TimescaleTick))
			writer.Add(tables.Measurements, measurement)
		}

		So(writer.CommitReady(ctx, true), ShouldBeNil)

		Convey("Timeline bounds on tick only", func() {
			rows, err := drainSeq(catalog.Timeline(ctx, epoch, "BTC/USD", 1, 4))
			So(err, ShouldBeNil)
			So(len(rows), ShouldEqual, 0)

			rows, err = drainSeq(catalog.Timeline(ctx, epoch, "BTC/USD", 101, 103))
			So(err, ShouldBeNil)
			So(len(rows), ShouldEqual, 3)

			for _, row := range rows {
				So(row.Tick, ShouldBeBetweenOrEqual, 101, 103)
			}
		})
	})
}
