package grid

import (
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"strconv"
	"testing"
	"time"
)

func TestSpaceImpulse(t *testing.T) {
	Convey("Only resolved multi-quantity regions activate agents", t, func() {
		for _, regime := range []string{"coherent", "inverse", "immature", "singleton", "independent"} {
			Convey(regime, func() {
				space := NewSpace()
				var impulse Impulse
				for index := range 128 {
					at := time.Unix(int64(index+1), 0)
					measurement := data.NewMeasurement[float64]("fixture", "market", "source", at, time.Unix(1, 0))
					// Repeated observations with known high SNR; maturity derives from the
					// actual fixture sample count, not a manually assigned maturity field.
					measurement.Metadata = map[string]float64{data.MetadataSupport: float64(index + 1), data.MetadataMahalanobisSNR: 100}
					if regime == "immature" {
						measurement.Metadata[data.MetadataSupport] = 1
					}
					first := float64(index%2)*2 - 1
					measurement.PutMetric(data.Metric[float64]{Label: "first", Raw: first})
					if regime != "singleton" {
						second := first
						if regime == "inverse" {
							second = -first
						}
						if regime == "independent" {
							second = float64((index/2)%2)*2 - 1
						}
						measurement.PutMetric(data.Metric[float64]{Label: "second", Raw: second})
					}
					So(space.Step([]*data.Measurement[float64]{measurement}), ShouldBeNil)
					var err error
					impulse, err = space.Impulse("market", at, measurement.From)
					So(err, ShouldBeNil)
					if index == 0 {
						So(impulse.Ready, ShouldBeFalse)
					}
				}
				So(impulse.Ready, ShouldEqual, regime == "coherent" || regime == "inverse")
				if impulse.Ready {
					original := impulse.Regions[0]
					_, _, err := space.Regions("market")
					So(err, ShouldBeNil)
					So(impulse.Regions[0], ShouldResemble, original)
				}
			})
		}
	})
}

func BenchmarkSpaceImpulse(b *testing.B) {
	space := NewSpace()
	measurement := data.NewMeasurement[float64]("bench", "market", "source", time.Unix(1, 0), time.Unix(1, 0))
	measurement.Metadata = map[string]float64{data.MetadataSupport: 100, data.MetadataMahalanobisSNR: 100}
	for index := range 64 {
		measurement.PutMetric(data.Metric[float64]{Label: strconv.Itoa(index), Raw: 1})
	}
	for index := range 64 {
		for label, metric := range measurement.Metrics {
			metric.Raw = float64(index%2)*2 - 1
			measurement.Metrics[label] = metric
		}
		if err := space.Step([]*data.Measurement[float64]{measurement}); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := space.Impulse("market", measurement.At, measurement.From); err != nil {
			b.Fatal(err)
		}
	}
}
