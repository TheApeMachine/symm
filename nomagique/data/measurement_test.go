package data

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMeasurementFinalize(t *testing.T) {
	Convey("Given a measurement without historical support", t, func() {
		measurement := NewMeasurement("source", map[string]Metric[float64]{})
		measurement.Label, measurement.At, measurement.From = "label", time.Now(), time.Now()
		measurement.Finalize()

		Convey("It should be whole with Maturity 1 and undefined SNR 0", func() {
			So(measurement.Maturity, ShouldEqual, 1.0)
			So(measurement.SNR, ShouldEqual, 0.0)
		})
	})

	Convey("Given a measurement with scalar divergence and noise variance", t, func() {
		measurement := NewMeasurement("source", map[string]Metric[float64]{})
		measurement.Label, measurement.At, measurement.From = "label", time.Now(), time.Now()
		measurement.Metadata = map[string]string{
			MetadataSupport:       "10",
			MetadataDivergence:    "4.0",
			MetadataNoiseVariance: "2.0",
		}
		measurement.Finalize()

		Convey("It should derive maturity 1 - 1/N and scalar SNR d^2 / sigma^2", func() {
			So(measurement.Maturity, ShouldAlmostEqual, 0.9, 1e-6)
			So(measurement.SNR, ShouldAlmostEqual, 8.0, 1e-6)
		})

		Convey("Reusing the measurement with missing noise clears its old SNR", func() {
			delete(measurement.Metadata, MetadataNoiseVariance)
			measurement.Finalize()
			So(measurement.Estimated, ShouldBeTrue)
			So(measurement.SNRDefined, ShouldBeFalse)
			So(measurement.SNR, ShouldEqual, 0)

			Convey("Fresh noise evidence restores a newly calculated ratio", func() {
				measurement.Metadata[MetadataNoiseVariance] = "4"
				measurement.Finalize()
				So(measurement.SNRDefined, ShouldBeTrue)
				So(measurement.SNR, ShouldEqual, 4)
			})
		})
	})

	Convey("Given a measurement with multivariate Mahalanobis SNR metadata", t, func() {
		measurement := NewMeasurement("source", map[string]Metric[float64]{})
		measurement.Label, measurement.At, measurement.From = "label", time.Now(), time.Now()
		measurement.Metadata = map[string]string{
			MetadataSupport:        "20",
			MetadataMahalanobisSNR: "5.5",
		}
		measurement.Finalize()

		Convey("It should derive maturity 1 - 1/N and prioritize Mahalanobis SNR", func() {
			So(measurement.Maturity, ShouldAlmostEqual, 0.95, 1e-6)
			So(measurement.SNR, ShouldAlmostEqual, 5.5, 1e-6)
		})
	})
}

func TestMeasurementClone(t *testing.T) {
	Convey("Given a measurement with metrics, metadata, and a peer", t, func() {
		peer := NewMeasurement("public", map[string]Metric[float64]{
			"bid": NewMetric[float64]("bid", UnitRate, TimescaleInstantaneous, 0, 1),
		})
		measurement := NewMeasurement("liquidity", map[string]Metric[float64]{
			"bid": NewMetric[float64]("bid", UnitRate, TimescaleInstantaneous, 0, 1),
		})
		measurement.Label = "BTC/USD"
		measurement.Metadata["peer-interest"] = "*"
		measurement.Provenance["channel"] = "ticker"
		measurement.Peers = []*Measurement[float64]{peer}
		measurement.Metrics["bid"] = measurement.Metrics["bid"].Write(100)

		clone := measurement.Clone()

		Convey("the clone is independent of later writes", func() {
			So(clone == measurement, ShouldBeFalse)
			So(clone.Label, ShouldEqual, "BTC/USD")
			So(clone.Metadata["peer-interest"], ShouldEqual, "*")
			So(clone.Provenance["channel"], ShouldEqual, "ticker")
			So(clone.Peers[0], ShouldEqual, peer)

			measurement.Metrics["bid"] = measurement.Metrics["bid"].Write(200)
			measurement.Metadata["peer-interest"] = "hawkes"
			So(clone.Metrics["bid"].Raw, ShouldEqual, 100)
			So(clone.Metadata["peer-interest"], ShouldEqual, "*")
		})
	})
}

func TestMeasurementPull(t *testing.T) {
	Convey("Given a feed measurement and an instrument measurement", t, func() {
		feed := NewMeasurement("public", map[string]Metric[float64]{
			"bid":    NewMetric[float64]("bid", UnitRate, TimescaleInstantaneous, 0, 1),
			"ask":    NewMetric[float64]("ask", UnitRate, TimescaleInstantaneous, 0, 1),
			"volume": NewMetric[float64]("volume", UnitCount, TimescaleInstantaneous, 0, 1),
		})
		feed.Label = "ETH/USD"
		feed.Provenance["side"] = "buy"
		feed.Metrics["bid"] = feed.Metrics["bid"].Write(10)
		feed.Metrics["ask"] = feed.Metrics["ask"].Write(11)
		feed.Metrics["volume"] = feed.Metrics["volume"].Write(5)

		owned := NewMeasurement("liquidity", map[string]Metric[float64]{
			"bid": NewMetric[float64]("bid", UnitRate, TimescaleInstantaneous, 0, 1),
			"ask": NewMetric[float64]("ask", UnitRate, TimescaleInstantaneous, 0, 1),
		})
		owned.Metadata["peer-interest"] = "*"
		owned.Pull(feed, "bid", "ask")

		Convey("named feed facts move without sharing the source map", func() {
			So(owned.Source, ShouldEqual, "liquidity")
			So(owned.Label, ShouldEqual, "ETH/USD")
			So(owned.Provenance["side"], ShouldEqual, "buy")
			So(owned.Metadata["peer-interest"], ShouldEqual, "*")
			So(owned.Metrics["bid"].Raw, ShouldEqual, 10)
			So(owned.Metrics["ask"].Raw, ShouldEqual, 11)
			_, hasVolume := owned.Metrics["volume"]
			So(hasVolume, ShouldBeFalse)

			owned.Metrics["bid"] = owned.Metrics["bid"].Write(99)
			So(feed.Metrics["bid"].Raw, ShouldEqual, 10)
		})
	})
}

func BenchmarkMeasurementFinalize(b *testing.B) {
	measurement := NewMeasurement("source", map[string]Metric[float64]{})
	measurement.Label, measurement.At, measurement.From = "label", time.Now(), time.Now()
	measurement.Metadata = map[string]string{
		MetadataSupport:        "25",
		MetadataMahalanobisSNR: "3.8",
	}

	b.ReportAllocs()

	for b.Loop() {
		measurement.Finalize()
	}
}
