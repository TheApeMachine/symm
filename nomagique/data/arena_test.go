package data

import (
	"math"
	"strings"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func stamp(measurement *Measurement, unix int64) {
	at := time.Unix(unix, 0).UTC()
	measurement.At = at
	measurement.From = at
}

func priceOf(measurement *Measurement) *Metric {
	for entry := range measurement.Read("price") {
		So(entry.Err, ShouldBeNil)
		return entry.Metric
	}
	return nil
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func TestArenaPriorTransfer(t *testing.T) {
	Convey("Given an ArenaOwner producing successive Measurements", t, func() {
		owner := NewArenaOwner("test-producer", 1024)
		Reset(func() { owner.Close() })

		Convey("samples, prediction, and metric Welford state transfer across allocs", func() {
			first := owner.NewMeasurement(1, "BTC/USD", "test-producer", 1, 1, nil)
			stamp(first, 1)
			first.Write(NewMetric("price", 100, UnitCurrency, TimescaleInstantaneous))

			So(first.locked(), ShouldBeTrue)
			So(first.samples, ShouldEqual, 1)
			firstPrice := priceOf(first)
			So(firstPrice, ShouldNotBeNil)
			So(firstPrice.center, ShouldEqual, 100)

			second := owner.NewMeasurement(1, "BTC/USD", "test-producer", 2, 2, nil)
			stamp(second, 2)
			So(second.samples, ShouldEqual, 1)
			So(second.prediction, ShouldEqual, first.prediction)

			second.Write(NewMetric("price", 110, UnitCurrency, TimescaleInstantaneous))

			So(second.samples, ShouldEqual, 2)
			secondPrice := priceOf(second)
			So(secondPrice, ShouldNotBeNil)
			So(secondPrice.center, ShouldEqual, 105)
			So(math.IsNaN(secondPrice.scale) || math.IsInf(secondPrice.scale, 0), ShouldBeFalse)
			So(secondPrice.scale, ShouldBeGreaterThan, 0)
			So(math.Abs(secondPrice.Standardized), ShouldBeGreaterThan, 0)
			So(second.Maturity(), ShouldBeGreaterThan, 0)

			third := owner.NewMeasurement(1, "BTC/USD", "test-producer", 3, 3, nil)
			stamp(third, 3)
			So(third.samples, ShouldEqual, 2)
			third.Write(NewMetric("price", 120, UnitCurrency, TimescaleInstantaneous))
			So(third.samples, ShouldEqual, 3)
			thirdPrice := priceOf(third)
			So(thirdPrice, ShouldNotBeNil)
			So(thirdPrice.center, ShouldEqual, 110)
		})

		Convey("priors are scoped by Label and do not cross symbols", func() {
			btc := owner.NewMeasurement(1, "BTC/USD", "test-producer", 1, 1, nil)
			stamp(btc, 1)
			btc.Write(NewMetric("price", 100, UnitCurrency, TimescaleInstantaneous))

			eth := owner.NewMeasurement(1, "ETH/USD", "test-producer", 2, 2, nil)
			stamp(eth, 2)
			So(eth.samples, ShouldEqual, 0)
			eth.Write(NewMetric("price", 50, UnitCurrency, TimescaleInstantaneous))
			So(eth.samples, ShouldEqual, 1)
			So(priceOf(eth).center, ShouldEqual, 50)

			btc2 := owner.NewMeasurement(1, "BTC/USD", "test-producer", 3, 3, nil)
			stamp(btc2, 3)
			So(btc2.samples, ShouldEqual, 1)
			btc2.Write(NewMetric("price", 110, UnitCurrency, TimescaleInstantaneous))
			So(btc2.samples, ShouldEqual, 2)
			So(priceOf(btc2).center, ShouldEqual, 105)
		})

		Convey("nil metadata and peers do not fail Require", func() {
			measurement := owner.NewMeasurement(1, "BTC/USD", "test-producer", 1, 1, nil)
			stamp(measurement, 1)
			So(measurement.metadata, ShouldBeNil)
			So(measurement.peers, ShouldBeNil)
			measurement.Write(NewMetric("price", 42, UnitCurrency, TimescaleInstantaneous))

			msg := errText(measurement.Error())
			So(strings.Contains(msg, "metadata is required"), ShouldBeFalse)
			So(strings.Contains(msg, "peers is required"), ShouldBeFalse)
		})
	})
}

func TestMeasurementOptionalRequire(t *testing.T) {
	Convey("Given a heap Measurement without metadata or peers", t, func() {
		measurement := NewMeasurement(1, "BTC/USD", "test", 1, 1)
		stamp(measurement, 1)

		Convey("finalize does not require metadata or peers", func() {
			measurement.Write(NewMetric("price", 42, UnitCurrency, TimescaleInstantaneous))
			msg := errText(measurement.Error())
			So(strings.Contains(msg, "metadata is required"), ShouldBeFalse)
			So(strings.Contains(msg, "peers is required"), ShouldBeFalse)
		})
	})
}

func TestArenaPriorReset(t *testing.T) {
	Convey("Given an ArenaOwner holding priors for two Labels", t, func() {
		owner := NewArenaOwner("test-producer", 1024)
		Reset(func() { owner.Close() })

		btc := owner.NewMeasurement(1, "BTC/USD", "test-producer", 1, 1, nil)
		stamp(btc, 1)
		btc.Write(NewMetric("price", 100, UnitCurrency, TimescaleInstantaneous))

		eth := owner.NewMeasurement(1, "ETH/USD", "test-producer", 2, 2, nil)
		stamp(eth, 2)
		eth.Write(NewMetric("price", 50, UnitCurrency, TimescaleInstantaneous))

		So(owner.priorFor("BTC/USD"), ShouldNotBeNil)
		So(owner.priorFor("ETH/USD"), ShouldNotBeNil)

		Convey("ResetPrior breaks transfer for that Label only", func() {
			owner.ResetPrior("BTC/USD")
			So(owner.priorFor("BTC/USD"), ShouldBeNil)
			So(owner.priorFor("ETH/USD"), ShouldNotBeNil)

			cold := owner.NewMeasurement(1, "BTC/USD", "test-producer", 3, 3, nil)
			stamp(cold, 3)
			So(cold.samples, ShouldEqual, 0)
			So(cold.prediction, ShouldEqual, 0)
			cold.Write(NewMetric("price", 200, UnitCurrency, TimescaleInstantaneous))
			So(cold.samples, ShouldEqual, 1)
			So(priceOf(cold).center, ShouldEqual, 200)

			warm := owner.NewMeasurement(1, "ETH/USD", "test-producer", 4, 4, nil)
			stamp(warm, 4)
			So(warm.samples, ShouldEqual, 1)
			warm.Write(NewMetric("price", 60, UnitCurrency, TimescaleInstantaneous))
			So(priceOf(warm).center, ShouldEqual, 55)

			Convey("and the new regime transfers from the post-reset prior", func() {
				next := owner.NewMeasurement(1, "BTC/USD", "test-producer", 5, 5, nil)
				stamp(next, 5)
				So(next.samples, ShouldEqual, 1)
				next.Write(NewMetric("price", 210, UnitCurrency, TimescaleInstantaneous))
				So(next.samples, ShouldEqual, 2)
				So(priceOf(next).center, ShouldEqual, 205)
			})
		})

		Convey("ResetPriors breaks transfer for every Label", func() {
			owner.ResetPriors()
			So(owner.priorFor("BTC/USD"), ShouldBeNil)
			So(owner.priorFor("ETH/USD"), ShouldBeNil)

			for _, label := range []string{"BTC/USD", "ETH/USD"} {
				cold := owner.NewMeasurement(1, label, "test-producer", 3, 3, nil)
				stamp(cold, 3)
				So(cold.samples, ShouldEqual, 0)
				cold.Write(NewMetric("price", 7, UnitCurrency, TimescaleInstantaneous))
				So(cold.samples, ShouldEqual, 1)
				So(priceOf(cold).center, ShouldEqual, 7)
			}
		})

		Convey("a Measurement in flight across ResetPrior does not resurrect the old regime", func() {
			inflight := owner.NewMeasurement(1, "BTC/USD", "test-producer", 3, 3, nil)
			stamp(inflight, 3)
			So(inflight.samples, ShouldEqual, 1)

			owner.ResetPrior("BTC/USD")

			inflight.Write(NewMetric("price", 110, UnitCurrency, TimescaleInstantaneous))
			So(inflight.locked(), ShouldBeTrue)
			So(inflight.samples, ShouldEqual, 2)
			So(priceOf(inflight).center, ShouldEqual, 105)
			So(owner.priorFor("BTC/USD"), ShouldBeNil)

			cold := owner.NewMeasurement(1, "BTC/USD", "test-producer", 4, 4, nil)
			stamp(cold, 4)
			So(cold.samples, ShouldEqual, 0)
		})

		Convey("a Measurement in flight across ResetPriors does not resurrect the old regime", func() {
			inflight := owner.NewMeasurement(1, "ETH/USD", "test-producer", 3, 3, nil)
			stamp(inflight, 3)

			owner.ResetPriors()

			inflight.Write(NewMetric("price", 60, UnitCurrency, TimescaleInstantaneous))
			So(owner.priorFor("ETH/USD"), ShouldBeNil)
		})

		Convey("a reset on another Label does not drop an in-flight capture", func() {
			inflight := owner.NewMeasurement(1, "ETH/USD", "test-producer", 3, 3, nil)
			stamp(inflight, 3)

			owner.ResetPrior("BTC/USD")

			inflight.Write(NewMetric("price", 60, UnitCurrency, TimescaleInstantaneous))
			prior := owner.priorFor("ETH/USD")
			So(prior, ShouldNotBeNil)
			So(prior.samples, ShouldEqual, 2)
		})

		Convey("seeds come from the prior pinned at alloc, not a later capture", func() {
			first := owner.NewMeasurement(1, "BTC/USD", "test-producer", 3, 3, nil)
			stamp(first, 3)
			second := owner.NewMeasurement(1, "BTC/USD", "test-producer", 4, 4, nil)
			stamp(second, 4)

			first.Write(NewMetric("price", 110, UnitCurrency, TimescaleInstantaneous))
			So(priceOf(first).center, ShouldEqual, 105)

			second.Write(NewMetric("price", 120, UnitCurrency, TimescaleInstantaneous))
			So(second.samples, ShouldEqual, 2)
			So(priceOf(second).center, ShouldEqual, 110)
		})
	})
}
