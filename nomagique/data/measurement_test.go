package data

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMeasurementFinalize(t *testing.T) {
	Convey("Given a measurement without historical support", t, func() {
		measurement := NewMeasurement[float64]("id-1", "label", "source", time.Now(), time.Now())
		measurement.Finalize()

		Convey("It should be whole with Maturity 1 and undefined SNR 0", func() {
			So(measurement.Maturity, ShouldEqual, 1.0)
			So(measurement.SNR, ShouldEqual, 0.0)
		})
	})

	Convey("Given a measurement with scalar divergence and noise variance", t, func() {
		measurement := NewMeasurement[float64]("id-2", "label", "source", time.Now(), time.Now())
		measurement.Metadata = map[string]float64{
			MetadataSupport:       10,
			MetadataDivergence:    4.0,
			MetadataNoiseVariance: 2.0,
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
				measurement.Metadata[MetadataNoiseVariance] = 4
				measurement.Finalize()
				So(measurement.SNRDefined, ShouldBeTrue)
				So(measurement.SNR, ShouldEqual, 4)
			})
		})
	})

	Convey("Given a measurement with multivariate Mahalanobis SNR metadata", t, func() {
		measurement := NewMeasurement[float64]("id-3", "label", "source", time.Now(), time.Now())
		measurement.Metadata = map[string]float64{
			MetadataSupport:        20,
			MetadataMahalanobisSNR: 5.5,
		}
		measurement.Finalize()

		Convey("It should derive maturity 1 - 1/N and prioritize Mahalanobis SNR", func() {
			So(measurement.Maturity, ShouldAlmostEqual, 0.95, 1e-6)
			So(measurement.SNR, ShouldAlmostEqual, 5.5, 1e-6)
		})
	})
}

func BenchmarkMeasurementFinalize(b *testing.B) {
	measurement := NewMeasurement[float64]("id-bench", "label", "source", time.Now(), time.Now())
	measurement.Metadata = map[string]float64{
		MetadataSupport:        25,
		MetadataMahalanobisSNR: 3.8,
	}

	b.ReportAllocs()

	for b.Loop() {
		measurement.Finalize()
	}
}
