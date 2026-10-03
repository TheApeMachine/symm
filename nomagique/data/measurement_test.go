package data

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMeasurementFinalize(t *testing.T) {
	Convey("Given a measurement without historical support", t, func() {
		measurement := NewMeasurement[float64]("source")
		measurement.Label, measurement.At, measurement.From = "label", time.Now(), time.Now()
		measurement.Finalize()

		Convey("It should be whole with Maturity 1 and undefined SNR 0", func() {
			So(measurement.Maturity, ShouldEqual, 1.0)
			So(measurement.SNR, ShouldEqual, 0.0)
		})
	})

	Convey("Given a measurement with scalar divergence and noise variance", t, func() {
		measurement := NewMeasurement[float64]("source")
		measurement.Label, measurement.At, measurement.From = "label", time.Now(), time.Now()
		measurement.SetMetadata(MetadataSupport, "10")
		measurement.SetMetadata(MetadataDivergence, "4.0")
		measurement.SetMetadata(MetadataNoiseVariance, "2.0")
		measurement.Finalize()

		Convey("It should derive maturity 1 - 1/N and scalar SNR d^2 / sigma^2", func() {
			So(measurement.Maturity, ShouldAlmostEqual, 0.9, 1e-6)
			So(measurement.SNR, ShouldAlmostEqual, 8.0, 1e-6)
		})

		Convey("Reusing the measurement with missing noise clears its old SNR", func() {
			measurement.DeleteMetadata(MetadataNoiseVariance)
			measurement.Finalize()
			So(measurement.Estimated, ShouldBeTrue)
			So(measurement.SNRDefined, ShouldBeFalse)
			So(measurement.SNR, ShouldEqual, 0)

			Convey("Fresh noise evidence restores a newly calculated ratio", func() {
				measurement.SetMetadata(MetadataNoiseVariance, "4")
				measurement.Finalize()
				So(measurement.SNRDefined, ShouldBeTrue)
				So(measurement.SNR, ShouldEqual, 4)
			})
		})
	})

	Convey("Given a measurement with multivariate Mahalanobis SNR metadata", t, func() {
		measurement := NewMeasurement[float64]("source")
		measurement.Label, measurement.At, measurement.From = "label", time.Now(), time.Now()
		measurement.SetMetadata(MetadataSupport, "20")
		measurement.SetMetadata(MetadataMahalanobisSNR, "5.5")
		measurement.Finalize()

		Convey("It should derive maturity 1 - 1/N and prioritize Mahalanobis SNR", func() {
			So(measurement.Maturity, ShouldAlmostEqual, 0.95, 1e-6)
			So(measurement.SNR, ShouldAlmostEqual, 5.5, 1e-6)
		})
	})
}

func TestMeasurementPeersLookup(t *testing.T) {
	Convey("Given a producer measurement with an immutable prior peer", t, func() {
		prior := NewMeasurement[float64]("websocket")
		prior.Label = "BTC/USD"
		prior.SetMetric("bid", NewMetric[float64]("bid", UnitPrice, TimescaleInstantaneous, 65000.75, 0.50).Write(65000.50))
		prior.SetMetric("ask", NewMetric[float64]("ask", UnitPrice, TimescaleInstantaneous, 65000.75, 0.50).Write(65001.00))

		out := NewMeasurement[float64]("liquidity")
		out.Label = prior.Label
		out.Peers = []*Measurement[float64]{prior}
		out.SetMetric("relative_spread", NewMetric[float64]("relative_spread", UnitRelativeSpread, TimescaleInstantaneous, 0.0001, 0.0001).Write(0.0001))

		Convey("LookupMetric reads local metrics first, then direct peers", func() {
			spread, hasSpread := out.LookupMetric("relative_spread")
			So(hasSpread, ShouldBeTrue)
			So(spread.Raw, ShouldEqual, 0.0001)

			bid, hasBid := out.LookupMetric("bid")
			So(hasBid, ShouldBeTrue)
			So(bid.Raw, ShouldEqual, 65000.50)

			ask, hasAsk := out.LookupMetric("ask")
			So(hasAsk, ShouldBeTrue)
			So(ask.Raw, ShouldEqual, 65001.00)

			_, hasMissing := out.LookupMetric("nonexistent")
			So(hasMissing, ShouldBeFalse)
		})

		Convey("LookupPeerMetric disambiguates source correctly", func() {
			bid, found := out.LookupPeerMetric("websocket", "bid")
			So(found, ShouldBeTrue)
			So(bid.Raw, ShouldEqual, 65000.50)

			_, notFound := out.LookupPeerMetric("hawkes", "bid")
			So(notFound, ShouldBeFalse)
		})

		Convey("Writes always target local storage without mutating peers", func() {
			out.SetMetric("bid", NewMetric[float64]("bid", UnitPrice, TimescaleInstantaneous, 99999.00, 1.0).Write(99999.00))
			So(out.GetMetric("bid").Raw, ShouldEqual, 99999.00)
			So(prior.GetMetric("bid").Raw, ShouldEqual, 65000.50)
		})
	})
}

func TestArenaOwnerGenerations(t *testing.T) {
	Convey("Given an ArenaOwner with capacity 4", t, func() {
		owner := NewArenaOwner(4)
		gen0 := owner.CurrentGeneration()
		So(gen0, ShouldNotBeNil)
		So(gen0.RefCount(), ShouldEqual, 1)

		m0 := owner.NewMeasurement("test")
		So(m0.Source, ShouldEqual, "test")

		// 3 more allocations reach capacity threshold
		owner.NewMeasurement("test")
		owner.NewMeasurement("test")
		owner.NewMeasurement("test")

		gen1 := owner.CurrentGeneration()
		So(gen1 != gen0, ShouldBeTrue)
		// gen0 is now previous generation; when rotating to gen2, gen0 is sealed and released
		owner.Rotate()
		So(gen0.IsFreed(), ShouldBeTrue)

		owner.Close()
	})
}

func BenchmarkMeasurementFinalize(b *testing.B) {
	measurement := NewMeasurement[float64]("source")
	measurement.Label, measurement.At, measurement.From = "label", time.Now(), time.Now()
	measurement.SetMetadata(MetadataSupport, "25")
	measurement.SetMetadata(MetadataMahalanobisSNR, "3.8")

	b.ReportAllocs()

	for b.Loop() {
		measurement.Finalize()
	}
}
