package strategy

import (
	"testing"
	"time"
	"unsafe"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/learning/associative"
	"github.com/theapemachine/symm/nomagique/learning/associative/grid"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestAgent(t *testing.T) {
	Convey("Given an autonomous Agent with private grid.Space", t, func() {
		engine := cognition.NewEngine(cognition.Config{})
		agent := NewAgent(0, true, engine, 8)
		So(agent, ShouldNotBeNil)
		So(agent.ID(), ShouldEqual, 0)
		So(agent.IsLive(), ShouldBeTrue)
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
	})
}

/*
mustObserve records one graded association for a test learner.
*/
func mustObserve(t *testing.T, engine core.Primitive, context, class []byte, feedback ...float64) {
	t.Helper()

	assoc := cognition.Association{Context: context, Class: class}

	if len(feedback) > 0 {
		assoc.Feedback = feedback[0]
		assoc.Graded = true
	}

	if err := observeContext(engine, assoc); err != nil {
		t.Fatal(err)
	}
}

/*
mustEvaluate classifies one context for a test learner.
*/
func mustEvaluate(t *testing.T, engine core.Primitive, context []byte) cognition.Evaluation {
	t.Helper()

	evaluation, err := evaluateContext(engine, context)
	if err != nil {
		t.Fatal(err)
	}

	return evaluation
}

/*
mustTree reads the engine's current trie for a test learner.
*/
func mustTree(t *testing.T, engine core.Primitive) *iradix.Tree[[]byte] {
	t.Helper()

	tree, err := engineTree(engine)
	if err != nil {
		t.Fatal(err)
	}

	return tree
}

/*
sequenceOf drives the temporal context primitive with one impulse and reads the
encoded sequence it produced.
*/
func sequenceOf(t *testing.T, context core.Primitive, impulse grid.Impulse) []byte {
	evaluation := transport.NewEvaluate(context)

	for out := range evaluation.Next(transport.NewOne(unsafe.Pointer(
		&associative.ContextCommand{Encode: &impulse},
	)).Next(nil)) {
		return (*associative.ContextResult)(out).Sequence
	}

	if err := evaluation.Error(); err != nil {
		t.Fatal(err)
	}

	return nil
}
