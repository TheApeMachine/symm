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
		sig1.WriteMetric("delta", 100.0)
		writer.Add("measurements", data.Publication{Measurement: sig1})

		sig2 := data.NewMeasurement[float64]("cvd", nil)
		sig2.Label = "BTC/USD"
		sig2.SeqIdx = 20
		sig2.At = at.Add(time.Second)
		sig2.Maturity = 1.0
		sig2.WriteMetric("delta", 150.0)
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
