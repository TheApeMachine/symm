package resonance

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/types"
)

/*
TestResonanceWire mirrors ResonanceWire: the dashboard frame must carry the
task head's horizon call as a first-class field, not only as a derived verdict
direction. The prediction chart binds data-paint="taskForecast"; without this
field the tile reads absent and renders "—" forever. Because the forward curve
is cumulative per horizon, the call is the curve's last element: the prediction
of where the move lands after the supported horizon, which is what the user
reads as "the forecast".
*/
func TestResonanceWire(t *testing.T) {
	Convey("Given a predictive coder with a settled manifold", t, func() {
		coder := learning.NewPredictiveCoder(learning.PredictiveCoderConfig{
			CustomArch: []int{11, 44, 22, 11},
			MaxHorizon: 8,
			Target:     learning.DirectionalTarget(0.01),
			Pace:       0.03,
			Learn:      true,
		})

		Convey("ResonanceWire publishes the forward curve head as taskForecast", func() {
			out := learning.PredictiveOutput{ForwardCurve: []float64{0.0125}}
			frame := ResonanceWire("BTC/USD", time.Now(), testArtifact(coder, out))

			So(frame.TaskForecast, ShouldAlmostEqual, 0.0125, 1e-9)
			So(frame.Verdict.Direction, ShouldEqual, 1.0)
		})

		Convey("the horizon call is the curve's last element, not its first", func() {
			// The move is flat for one tick and resolves up over the horizon;
			// the call must reflect the horizon outcome.
			out := learning.PredictiveOutput{ForwardCurve: []float64{0.001, -0.002, 0.03}}
			frame := ResonanceWire("BTC/USD", time.Now(), testArtifact(coder, out))

			So(frame.TaskForecast, ShouldAlmostEqual, 0.03, 1e-9)
			So(frame.Verdict.Direction, ShouldEqual, 1.0)
		})

		Convey("a downward horizon head yields a negative taskForecast and a down call", func() {
			out := learning.PredictiveOutput{ForwardCurve: []float64{-0.008}}
			frame := ResonanceWire("BTC/USD", time.Now(), testArtifact(coder, out))

			So(frame.TaskForecast, ShouldAlmostEqual, -0.008, 1e-9)
			So(frame.Verdict.Direction, ShouldEqual, -1.0)
		})

		Convey("an empty forward curve yields a zero taskForecast", func() {
			frame := ResonanceWire(
				"BTC/USD", time.Now(), testArtifact(coder, learning.PredictiveOutput{}),
			)

			So(frame.TaskForecast, ShouldEqual, 0.0)
			So(frame.Verdict.Direction, ShouldEqual, 0.0)
		})
	})
}

// testArtifact folds a predictive output into the enriched domain payload the
// workspace observer projects into the dashboard frame.
func testArtifact(coder *learning.PredictiveCoder, out learning.PredictiveOutput) types.ResonanceArtifact {
	artifact := types.ResonanceArtifact{
		Symbol:           "BTC/USD",
		At:               time.Now(),
		Manifold:         coder.Manifold(),
		Dynamics:         out.Dynamics,
		ForwardCurve:     out.ForwardCurve,
		ForwardRetention: out.ForwardRetention,
		SupportedHorizon: out.SupportedHorizon,
		Calibrated:       out.Calibrated,
		ResolvedSteps:    out.ResolvedSteps,
		Readout:          out.Readout,
		Confidence:       out.Confidence,
	}

	if out.LastResolution != nil {
		artifact.LastResolutionTarget = out.LastResolution.Target
		artifact.LastResolutionError = out.LastResolution.Error
	}

	return artifact
}
