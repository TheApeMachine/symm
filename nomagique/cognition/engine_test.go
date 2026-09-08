package cognition

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestEngineObserveAndEvaluate(t *testing.T) {
	Convey("A cognitive engine classifies the basin a context was observed in", t, func() {
		engine := NewEngine(DefaultConfig())
		context := []byte{0x01, 0x02, 0x03}

		engine.Observe(context, []byte("enter"))
		engine.Observe(context, []byte("enter"))
		engine.Observe(context, []byte("enter"))
		engine.Observe(context, []byte("wait"))

		evaluation := engine.Evaluate(context)
		So(evaluation.WinnerClass, ShouldEqual, "enter")
		So(evaluation.Confidence, ShouldBeGreaterThan, 0)
		So(evaluation.Surprisal, ShouldBeGreaterThanOrEqualTo, 0)
		So(evaluation.IsBreak, ShouldBeFalse)
	})
}

func TestEngineUnseenContextIsSurprising(t *testing.T) {
	Convey("An unseen context carries Dirichlet-baseline surprisal", t, func() {
		engine := NewEngine(DefaultConfig())

		evaluation := engine.Evaluate([]byte{0x09, 0x08})
		So(evaluation.WinnerClass, ShouldEqual, "")
		So(evaluation.Surprisal, ShouldBeGreaterThan, 0)
		So(evaluation.IsBreak, ShouldBeTrue)
	})
}

func TestEngineEmptyContextBreaks(t *testing.T) {
	Convey("An empty context cannot be matched and reports a break", t, func() {
		engine := NewEngine(DefaultConfig())

		evaluation := engine.Evaluate(nil)
		So(evaluation.IsBreak, ShouldBeTrue)
	})
}
