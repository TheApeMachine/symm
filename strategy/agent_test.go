package strategy

import (
	"math/rand"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/learning/associative/grid"
	"github.com/theapemachine/symm/types"
)

func TestAgent(t *testing.T) {
	Convey("Given an autonomous Agent with private grid.Space", t, func() {
		engine := cognition.NewEngine(cognition.Config{})
		rng := rand.New(rand.NewSource(42))
		agent := NewAgent(1, false, engine, 8, rng)

		So(agent, ShouldNotBeNil)
		So(agent.ID(), ShouldEqual, 1)
		So(agent.IsLive(), ShouldBeFalse)
		So(agent.Space(), ShouldNotBeNil)
		So(agent.Engine(), ShouldEqual, engine)

		Convey("When stepping with multi-signal measurements", func() {
			measurement := data.NewMeasurement[float64]("resonance", nil)
			measurement.Label, measurement.At, measurement.From = "BTC/USD", time.Now().UTC(), time.Now().UTC()
			measurement.Metrics["forward/0"] = data.Metric[float64]{Label: "forward/0", Raw: 1.5}
			measurement.Metrics["confidence"] = data.Metric[float64]{Label: "confidence", Raw: 0.9}
			measurement.Maturity = 1.0
			measurement.SNR = 10.0
			measurement.SNRDefined = true

			impulse, err := agent.Step([]*data.Measurement[float64]{measurement}, "BTC/USD")
			So(err, ShouldBeNil)
			So(impulse.Label, ShouldEqual, "BTC/USD")
			So(impulse.Ready, ShouldBeFalse)
		})

		Convey("When choosing actions in unprimed state", func() {
			emptyImpulse := grid.Impulse{}

			decisionFlat, errFlat := agent.ChooseAction(emptyImpulse, false)
			So(errFlat, ShouldBeNil)
			So(decisionFlat.Action, ShouldEqual, ActionWait)

			decisionHolding, errHolding := agent.ChooseAction(emptyImpulse, true)
			So(errHolding, ShouldBeNil)
			So(decisionHolding.Action, ShouldEqual, ActionHold)
		})

		Convey("When rehearsing an excursion fragment", func() {
			measurementA := data.NewMeasurement[float64]("cvd", nil)
			measurementA.Label, measurementA.At, measurementA.From = "BTC/USD", time.Now().UTC(), time.Now().UTC()
			measurementA.Metrics["signed"] = data.Metric[float64]{Label: "signed", Raw: 1.0}

			measurementB := data.NewMeasurement[float64]("cvd", nil)
			measurementB.Label, measurementB.At, measurementB.From = "BTC/USD", time.Now().UTC(), time.Now().UTC()
			measurementB.Metrics["signed"] = data.Metric[float64]{Label: "signed", Raw: 2.0}

			measurementC := data.NewMeasurement[float64]("cvd", nil)
			measurementC.Label, measurementC.At, measurementC.From = "BTC/USD", time.Now().UTC(), time.Now().UTC()
			measurementC.Metrics["signed"] = data.Metric[float64]{Label: "signed", Raw: 3.0}

			positiveFragment := types.ReplayFragment{
				Frames: [][]*data.Measurement[float64]{
					{measurementA},
					{measurementB},
					{measurementC},
				},
				Symbol:        "BTC/USD",
				AnchorIndex:   1,
				ExtremumIndex: 2,
			}

			agent.IngestReplay(positiveFragment, 0)
			stepped, err := agent.RehearseChild()
			So(err, ShouldBeNil)
			So(stepped, ShouldBeGreaterThanOrEqualTo, 1)

			marks := agent.LastMarks()
			if len(marks) > 0 {
				So(marks[0].Kind, ShouldNotBeEmpty)
			}

		})

		Convey("When rehearsing a negative non-event fragment (AT-13, AT-14)", func() {
			measurementA := data.NewMeasurement[float64]("cvd", nil)
			measurementA.Label, measurementA.At, measurementA.From = "BTC/USD", time.Now().UTC(), time.Now().UTC()
			measurementA.Metrics["signed"] = data.Metric[float64]{Label: "signed", Raw: 0.1}

			measurementB := data.NewMeasurement[float64]("cvd", nil)
			measurementB.Label, measurementB.At, measurementB.From = "BTC/USD", time.Now().UTC(), time.Now().UTC()
			measurementB.Metrics["signed"] = data.Metric[float64]{Label: "signed", Raw: 0.05}

			negativeFragment := types.ReplayFragment{
				Frames: [][]*data.Measurement[float64]{
					{measurementA},
					{measurementB},
				},
				Symbol:        "BTC/USD",
				AnchorIndex:   -1,
				ExtremumIndex: -1,
			}

			agent.IngestReplay(negativeFragment, 0)
			stepped, err := agent.RehearseChild()
			So(err, ShouldBeNil)
			So(stepped, ShouldBeGreaterThanOrEqualTo, 1)
		})
	})
}
