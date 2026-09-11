package strategy

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestAgent(t *testing.T) {
	Convey("Given an autonomous Agent with private grid.Space", t, func() {
		engine := cognition.NewEngine(cognition.DefaultConfig())
		agent := NewAgent(0, true, engine, 8)
		So(agent, ShouldNotBeNil)
		So(agent.ID(), ShouldEqual, 0)
		So(agent.IsLive(), ShouldBeTrue)
		So(agent.Space(), ShouldNotBeNil)
		So(agent.Engine(), ShouldEqual, engine)

		Convey("When stepping with multi-signal measurements", func() {
			measurement := data.NewMeasurement[float64]("m1", "BTC/USD", "resonance", time.Now().UTC(), time.Now().UTC())
			measurement.PutMetric(data.Metric[float64]{Label: "forward/0", Raw: 1.5})
			measurement.PutMetric(data.Metric[float64]{Label: "confidence", Raw: 0.9})
			measurement.Maturity = 1.0
			measurement.SNR = 10.0
			measurement.SNRDefined = true

			impulse, err := agent.Step([]*data.Measurement[float64]{measurement}, "BTC/USD")
			So(err, ShouldBeNil)
			So(impulse.Label, ShouldEqual, "BTC/USD")
			So(impulse.Ready, ShouldBeFalse)
		})
	})
}
