package system

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestDiagnosticStampsItsComposition(t *testing.T) {
	Convey("Given a Diagnostic told where it sits in the composition", t, func() {
		diagnostic := NewDiagnostic("ticker.pumpdump")
		diagnostic.Compose("ticker", 2)

		envelope := diagnostic.Step(&types.Envelope{})

		Convey("Its stamp carries the ring and handler group it runs in", func() {
			So(len(envelope.Boundaries), ShouldEqual, 1)
			So(envelope.Boundaries[0].Label, ShouldEqual, "ticker.pumpdump")
			So(envelope.Boundaries[0].Group, ShouldEqual, "ticker")
			So(envelope.Boundaries[0].Stage, ShouldEqual, int32(2))
		})
	})

	Convey("Given a Diagnostic no ring ever composed", t, func() {
		envelope := NewDiagnostic("loose.stage").Step(&types.Envelope{})

		Convey("It stamps an empty ring rather than inventing one", func() {
			So(envelope.Boundaries[0].Group, ShouldEqual, "")
			So(envelope.Boundaries[0].Stage, ShouldEqual, int32(0))
		})
	})
}

type countingNode struct{ calls int }

func (node *countingNode) Step(envelope *types.Envelope) *types.Envelope {
	node.calls++

	return envelope
}

func TestTracedForwardsItsComposition(t *testing.T) {
	Convey("Given a Traced signal inside a ring", t, func() {
		inner := &countingNode{}
		traced := NewTraced("trade.hawkes", inner)
		traced.Compose("trade", 1)

		envelope := traced.StepBacklog(&types.Envelope{}, 7)

		Convey("The wrapped node still runs before the label is stamped", func() {
			So(inner.calls, ShouldEqual, 1)
		})

		Convey("And the stamp carries the ring, stage and real ring pressure", func() {
			So(envelope.Boundaries[0].Group, ShouldEqual, "trade")
			So(envelope.Boundaries[0].Stage, ShouldEqual, int32(1))
			So(envelope.Boundaries[0].Backlog, ShouldEqual, int64(7))
		})
	})
}
