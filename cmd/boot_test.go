package cmd

import (
	"context"
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	nmtypes "github.com/theapemachine/symm/nomagique/types"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
)

/*
runnableFixture provides one controllable lifecycle component for System.Run.
*/
type runnableFixture struct {
	ctx context.Context
	err error
}

func (fixture *runnableFixture) Name() string { return "fixture" }
func (fixture *runnableFixture) Error() error { return fixture.err }

func (fixture *runnableFixture) Run() error {
	if fixture.err != nil {
		return fixture.err
	}

	<-fixture.ctx.Done()

	return fixture.ctx.Err()
}

func TestSystemRun(t *testing.T) {
	Convey("Given one failed system and one context-bound peer", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		failure := errors.New("fixture failed")
		system := &System{
			ctx:    ctx,
			cancel: cancel,
			Systems: []Runnable{
				&runnableFixture{ctx: ctx, err: failure},
				&runnableFixture{ctx: ctx},
			},
		}

		err := system.Run()

		Convey("It should cancel its peer and return the originating error", func() {
			So(errors.Is(err, failure), ShouldBeTrue)
			So(ctx.Err(), ShouldEqual, context.Canceled)
		})
	})

	Convey("Given expected component cancellation after system shutdown", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		system := &System{ctx: ctx, cancel: cancel}
		cancel()

		system.fail(context.Canceled)

		Convey("It should not retain shutdown as a system failure", func() {
			So(system.Error(), ShouldBeNil)
		})
	})
}

func TestSystemClose(t *testing.T) {
	Convey("Given resources recorded in acquisition order", t, func() {
		closed := make([]int, 0, 3)
		system := &System{closers: []func() error{
			func() error {
				closed = append(closed, 1)
				return nil
			},
			func() error {
				closed = append(closed, 2)
				return nil
			},
			func() error {
				closed = append(closed, 3)
				return nil
			},
		}}

		err := system.Close()

		Convey("It should release every resource in reverse acquisition order", func() {
			So(err, ShouldBeNil)
			So(closed, ShouldResemble, []int{3, 2, 1})
		})
	})
}

func TestMeasurementWireDeterministicOrder(t *testing.T) {
	Convey("Given a measurement with unordered metrics", t, func() {
		measurement := &nmtypes.Measurement{
			Source: "cvd",
			Symbol: "BTC/USD",
			Metrics: map[string]*nmtypes.Metric[float64]{
				"zebra": nmtypes.NewMetric(
					"zebra", 1.0, nmtypes.Descriptor{Unit: nmtypes.UnitDimensionless, Timescale: nmtypes.TimescaleInstantaneous},
				),
				"alpha": nmtypes.NewMetric(
					"alpha", 2.0, nmtypes.Descriptor{Unit: nmtypes.UnitDimensionless, Timescale: nmtypes.TimescaleInstantaneous},
				),
				"midpoint": nmtypes.NewMetric(
					"midpoint", 3.0, nmtypes.Descriptor{Unit: nmtypes.UnitDimensionless, Timescale: nmtypes.TimescaleInstantaneous},
				),
			},
		}

		Convey("the wire row emits metrics in sorted name order", func() {
			row := measurementWire(measurement)
			So(row, ShouldNotBeNil)
			So(row.Metrics, ShouldHaveLength, 3)
			So(row.Metrics[0].Name, ShouldEqual, "alpha")
			So(row.Metrics[1].Name, ShouldEqual, "midpoint")
			So(row.Metrics[2].Name, ShouldEqual, "zebra")
		})

		Convey("a defined SNR serializes as an 'snr' metric on the wire", func() {
			measurement.SNR = 12.5
			measurement.SNRDefined = true

			row := measurementWire(measurement)
			So(row, ShouldNotBeNil)
			So(row.Metrics, ShouldHaveLength, 4)

			var snr *wire.MetricT

			for _, metric := range row.Metrics {
				if metric.Name == "snr" {
					snr = metric
				}
			}

			So(snr, ShouldNotBeNil)
			So(snr.Raw, ShouldEqual, 12.5)
		})

		Convey("an undefined SNR produces no 'snr' metric", func() {
			measurement.SNRDefined = false

			row := measurementWire(measurement)
			So(row, ShouldNotBeNil)

			for _, metric := range row.Metrics {
				So(metric.Name, ShouldNotEqual, "snr")
			}
		})

		Convey("a map entry named 'snr' never duplicates the dedicated SNR", func() {
			measurement.SNR = 12.5
			measurement.SNRDefined = true
			measurement.Metrics["snr"] = nmtypes.NewMetric(
				"snr", 99.0, nmtypes.Descriptor{Unit: nmtypes.UnitDimensionless, Timescale: nmtypes.TimescaleInstantaneous},
			)

			row := measurementWire(measurement)

			So(row, ShouldNotBeNil)

			var snrCount int
			var snr *wire.MetricT

			for _, metric := range row.Metrics {
				if metric.Name == "snr" {
					snrCount++
					snr = metric
				}
			}

			So(snrCount, ShouldEqual, 1)
			So(snr, ShouldNotBeNil)
			So(snr.Raw, ShouldEqual, 12.5)
		})

		Convey("nil measurements produce no row", func() {
			So(measurementWire(nil), ShouldBeNil)
		})
	})
}
