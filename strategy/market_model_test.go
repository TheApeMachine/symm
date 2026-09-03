package strategy

import (
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/types"
)

func TestNewResonanceMarketModel(t *testing.T) {
	Convey("Given a calibrated resonance artifact with a multi-step horizon", t, func() {
		artifact := &types.ResonanceArtifact{
			Calibrated:       true,
			SupportedHorizon: 16,
			Confidence:       0.8,
			Forecast: &types.ResonanceReturnForecast{
				Call:    1,
				Horizon: 16,
				Distribution: learning.RLSOutput{
					Ready: true,
					Scale: 0.04,
				},
			},
		}

		model, ready := newResonanceMarketModel(artifact, time.Second)

		Convey("the model is ready and scales horizon variance to 1-step volatility", func() {
			So(ready, ShouldBeTrue)
			So(model, ShouldNotBeNil)

			expectedVolatility := 0.04 / math.Sqrt(16) // 0.01
			So(model.volatility, ShouldAlmostEqual, expectedVolatility, 1e-6)
			So(model.drift, ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given an uncalibrated resonance artifact", t, func() {
		artifact := &types.ResonanceArtifact{
			Calibrated: false,
		}

		model, ready := newResonanceMarketModel(artifact, time.Second)

		Convey("the model is not ready", func() {
			So(ready, ShouldBeFalse)
			So(model, ShouldBeNil)
		})
	})
}
