package grid

import (
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"strconv"
	"testing"
	"time"
)

func TestSpaceImpulse(t *testing.T) {
	Convey("Formation readiness belongs to the settled grid, not current activity", t, func() {
		for _, regime := range []string{"coherent", "inverse", "immature", "singleton", "independent"} {
			Convey(regime, func() {
				space := NewSpace().(*Space)
				var impulse Impulse
				for index := range 4096 {
					at := time.Unix(int64(index+1), 0)
					measurement := data.NewMeasurement[float64]("source", nil)
					measurement.Label, measurement.At, measurement.From = "market", at, time.Unix(1, 0)
					// Repeated observations with known high SNR; maturity derives from the
					// actual fixture sample count, not a manually assigned maturity field.
					measurement.Metadata = map[string]float64{data.MetadataSupport: float64(index + 1), data.MetadataMahalanobisSNR: 100}
					if regime == "immature" {
						measurement.Metadata[data.MetadataSupport] = 1
					}
					first := float64(index%2)*2 - 1
					measurement.Metrics["first"] = data.Metric[float64]{Label: "first", Raw: first}
					if regime != "singleton" {
						second := first
						if regime == "inverse" {
							second = -first
						}
						if regime == "independent" {
							second = float64((index/2)%2)*2 - 1
						}
						measurement.Metrics["second"] = data.Metric[float64]{Label: "second", Raw: second}
					}
					So(space.step([]*data.Measurement[float64]{measurement}), ShouldBeNil)
					var err error
					impulse, err = space.impulse("market", at, measurement.From)
					So(err, ShouldBeNil)
					if impulse.Ready {
						break
					}
					if index == 0 {
						So(impulse.Ready, ShouldBeFalse)
					}
				}
				So(impulse.Ready, ShouldEqual, regime == "coherent" || regime == "inverse" || regime == "independent")
				if impulse.Ready {
					original := impulse.Regions[0]
					_, _, err := space.regionsOf("market")
					So(err, ShouldBeNil)
					So(impulse.Regions[0], ShouldResemble, original)
				}
			})
		}
	})
}

func BenchmarkSpaceImpulse(b *testing.B) {
	space := NewSpace().(*Space)
	measurement := data.NewMeasurement[float64]("source", nil)
	measurement.Label, measurement.At, measurement.From = "market", time.Unix(1, 0), time.Unix(1, 0)
	measurement.Metadata = map[string]float64{data.MetadataSupport: 100, data.MetadataMahalanobisSNR: 100}
	for index := range 64 {
		measurement.Metrics[strconv.Itoa(index)] = data.Metric[float64]{Label: strconv.Itoa(index), Raw: 1}
	}
	for index := range 64 {
		for label, metric := range measurement.Metrics {
			metric.Raw = float64(index%2)*2 - 1
			measurement.Metrics[label] = metric
		}
		if err := space.step([]*data.Measurement[float64]{measurement}); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := space.impulse("market", measurement.At, measurement.From); err != nil {
			b.Fatal(err)
		}
	}
}
