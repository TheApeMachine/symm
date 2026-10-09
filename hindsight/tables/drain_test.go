package tables_test

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/nomagique/data"
	nmruntime "github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/tests/tablestest"
)

func TestCatalog_Drain(t *testing.T) {
	Convey("Given nil catalog or tee", t, func() {
		ctx := context.Background()
		catalog := tablestest.New(t)
		epoch := int64(300)

		So(catalog.Drain(ctx, epoch, nil), ShouldBeNil)
	})

	Convey("Given an active StoreTee and Iceberg catalog", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		catalog := tablestest.New(t)
		epoch := int64(301)
		tee := hindsight.NewStoreTee(ctx, "testStoreTee")
		tee.Transition(nmruntime.READY)

		measurement := data.NewMeasurement(epoch, "BTC/USD", "spot:trade", 1, 1)
		measurement.At = time.Now().UTC()
		measurement.From = measurement.At
		measurement.Write(data.NewMetric("price", 65000.0, data.UnitCurrency, data.TimescaleTick))
		tee.Push(measurement)

		drainErr := make(chan error, 1)
		go func() {
			drainErr <- catalog.Drain(ctx, epoch, tee)
		}()

		// Allow drain loop to ingest the measurement from tee
		time.Sleep(100 * time.Millisecond)

		// Trigger graceful shutdown
		cancel()

		select {
		case err := <-drainErr:
			So(err, ShouldBeNil)
		case <-time.After(2 * time.Second):
			t.Fatal("drain did not complete within timeout")
		}

		readMeasurements, err := drainSeq(catalog.Trades(context.Background(), epoch))
		So(err, ShouldBeNil)
		So(len(readMeasurements), ShouldEqual, 1)
		So(readMeasurements[0].Label, ShouldEqual, "BTC/USD")
	})
}
