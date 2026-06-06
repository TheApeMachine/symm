package types

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
)

func TestSourceTypeString(t *testing.T) {
	Convey("Given source types", t, func() {
		Convey("It should map to dashboard names", func() {
			So(SourceFluid.String(), ShouldEqual, "fluid")
			So(SourceHawkes.String(), ShouldEqual, "hawkes")
			So(SourceToxicity.String(), ShouldEqual, "toxicity")
		})

		Convey("It should return empty for SourceNone", func() {
			So(SourceNone.String(), ShouldBeBlank)
		})
	})
}

func TestMeasurementRequire(t *testing.T) {
	Convey("Given a complete measurement row", t, func() {
		measurement := Measurement{
			Symbol:     "BTC/EUR",
			Source:     SourceFluid,
			Category:   CategoryLaminar,
			Strength:   0.8,
			Confidence: 0.6,
			SNR:        1.5,
			Last:       50_000,
		}

		ctx := context.Background()
		pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
		defer pool.Close()

		Convey("It should satisfy the ingest contract", func() {
			So(measurement.Send(pool), ShouldBeNil)
		})
	})

	Convey("Given a row without a last price", t, func() {
		measurement := Measurement{
			Symbol:     "BTC/EUR",
			Source:     SourceFluid,
			Category:   CategoryLaminar,
			Strength:   0.8,
			Confidence: 0.6,
			SNR:        1.5,
		}

		ctx := context.Background()
		pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
		defer pool.Close()

		Convey("It should fail validation", func() {
			So(measurement.Send(pool), ShouldNotBeNil)
		})
	})
}

func TestMeasurementSend(t *testing.T) {
	Convey("Given a validated measurement and pool", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
		defer pool.Close()

		subscriber := pool.CreateBroadcastGroup("measurements", 0).
			Subscribe("test:measurement-send", 4)

		measurement := Measurement{
			Symbol:     "BTC/EUR",
			Source:     SourceFluid,
			Category:   CategoryLaminar,
			Strength:   0.8,
			Confidence: 0.6,
			SNR:        1.5,
			Last:       50_000,
		}

		Convey("It should publish on the measurements bus", func() {
			So(measurement.Send(pool), ShouldBeNil)

			select {
			case frame := <-subscriber.Incoming:
				published, ok := frame.Value.(Measurement)
				So(ok, ShouldBeTrue)
				So(published.Symbol, ShouldEqual, "BTC/EUR")
			default:
				t.Fatal("expected published measurement")
			}
		})
	})

	Convey("Given an incomplete measurement", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
		defer pool.Close()

		measurement := Measurement{
			Symbol:   "BTC/EUR",
			Source:   SourceFluid,
			Category: CategoryLaminar,
		}

		Convey("It should reject send", func() {
			So(measurement.Send(pool), ShouldNotBeNil)
		})
	})
}

func TestMeasurementFields(t *testing.T) {
	Convey("Given a measurement row", t, func() {
		row := Measurement{
			Symbol:     "BTC/EUR",
			Source:     SourceFluid,
			Category:   CategoryLaminar,
			Strength:   0.8,
			Confidence: 0.6,
			SNR:        1.5,
			Last:       50_000,
			SpreadBPS:  12,
		}

		Convey("It should carry symbol-scoped signal fields", func() {
			So(row.Symbol, ShouldEqual, "BTC/EUR")
			So(row.Source, ShouldEqual, SourceFluid)
			So(row.SNR, ShouldBeGreaterThan, row.Confidence)
		})
	})
}
