package correlation

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestCorrelationMetrics(t *testing.T) {
	Convey("Given cohort scores with their equation-defined domains", t, func() {
		metrics, valid := correlationMetrics(map[string]float64{
			"correlation":    0.8,
			"signed":         -0.6,
			"relativeEnergy": 1.5,
			"herdScore":      0.2,
			"alphaScore":     0.3,
			"noiseScore":     0.4,
			"stressScore":    0.5,
			"peakScore":      0.5,
			"strength":       0.5,
		})

		Convey("It should retain signed and relative evidence without fake scaling", func() {
			So(valid, ShouldBeTrue)
			So(*metrics[types.MetricKey(types.MetricSigned, types.SideNone)].Normalized,
				ShouldAlmostEqual, -0.6, 1e-12)
			So(*metrics[types.MetricKey(types.MetricRelativeEnergy, types.SideNone)].Normalized,
				ShouldAlmostEqual, 1.5, 1e-12)
		})
	})

	Convey("Given a missing cohort score", t, func() {
		metrics, valid := correlationMetrics(map[string]float64{})

		Convey("It should expose the incomplete bundle as invalid", func() {
			So(valid, ShouldBeFalse)
			So(metrics[types.MetricKey(types.MetricRelativeEnergy, types.SideNone)].Normalized,
				ShouldBeNil)
		})
	})
}

func BenchmarkCorrelationMetrics(b *testing.B) {
	scores := map[string]float64{
		"correlation": 0.8, "signed": 0.6, "relativeEnergy": 1.5,
		"herdScore": 0.2, "alphaScore": 0.3, "noiseScore": 0.4,
		"stressScore": 0.5, "peakScore": 0.5, "strength": 0.5,
	}

	b.ReportAllocs()

	for b.Loop() {
		_, _ = correlationMetrics(scores)
	}
}
