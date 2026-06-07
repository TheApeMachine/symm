package reasoning

import (
	"os"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func TestParseThoughtsPlaybook(t *testing.T) {
	Convey("Given the production playbook", t, func() {
		raw, err := os.ReadFile("../cfg/perspectives.yaml")
		So(err, ShouldBeNil)

		thoughts, err := ParseThoughts(raw)
		So(err, ShouldBeNil)

		Convey("It parses the pump-dip entry, exhaustion exit, and protective managers", func() {
			So(len(thoughts), ShouldEqual, 4)
		})

		Convey("The entry watches pump-dip price action while flat", func() {
			entry := thoughts[0]
			So(entry.Do.Type, ShouldEqual, ActionMarket)
			So(entry.Do.Fraction, ShouldEqual, 0.25)
			So(entry.When.All[0].Lifecycle, ShouldEqual, types.ObservationNotHolding)
			So(entry.When.All[1].Any, ShouldNotBeEmpty)
			So(entry.When.All[2].Subject, ShouldEqual, SubjectPrice)
			So(entry.When.All[2].Op, ShouldEqual, ComparisonFellBy)
		})

		Convey("The exit branch settles on reversal or exhaustion while holding", func() {
			exit := thoughts[1]
			So(exit.Do.Type, ShouldEqual, ActionSettlePosition)
			So(exit.When.All[1].Any, ShouldHaveLength, 2)
		})

		Convey("The protective managers arm stop-loss on has_started then trail while holding", func() {
			stop := thoughts[2]
			So(stop.Do.Type, ShouldEqual, ActionStopLoss)
			So(stop.Do.Offset, ShouldEqual, 0.012)
			So(stop.When.All[1].Lifecycle, ShouldEqual, types.ObservationHasStarted)

			trail := thoughts[3]
			So(trail.When.Lifecycle, ShouldEqual, types.ObservationHolding)
			So(trail.Do.Type, ShouldEqual, ActionTrailingStop)
			So(trail.Do.Offset, ShouldEqual, 0.01)
		})
	})
}

func TestMarshalThoughtsRoundTrips(t *testing.T) {
	Convey("Given the production playbook", t, func() {
		raw, err := os.ReadFile("../cfg/perspectives.yaml")
		So(err, ShouldBeNil)

		original, err := ParseThoughts(raw)
		So(err, ShouldBeNil)

		Convey("Marshalling and re-parsing reproduces the forest exactly", func() {
			encoded, err := MarshalThoughts(original, 2)
			So(err, ShouldBeNil)

			reparsed, err := ParseThoughts(encoded)
			So(err, ShouldBeNil)

			So(reparsed, ShouldResemble, original)
		})
	})
}
