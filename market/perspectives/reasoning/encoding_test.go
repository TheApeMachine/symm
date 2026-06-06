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

		Convey("It parses the tuned entry and protective manager", func() {
			So(len(thoughts), ShouldEqual, 2)
		})

		Convey("The entry watches extreme scarcity SNR while flat", func() {
			entry := thoughts[0]
			So(entry.Do.Type, ShouldEqual, ActionMarket)
			So(entry.Do.Fraction, ShouldEqual, 0.25)
			So(entry.When.All[0].Lifecycle, ShouldEqual, types.ObservationNotHolding)
			So(entry.When.All[1].Category, ShouldEqual, types.CategoryExtremeScarcity)
			So(entry.When.All[1].Unit, ShouldEqual, UnitSNR)
			So(entry.When.All[1].Value, ShouldEqual, 1.0)
		})

		Convey("The protective manager trails held positions", func() {
			manager := thoughts[1]
			So(manager.When.Lifecycle, ShouldEqual, types.ObservationHolding)
			So(manager.Do.Type, ShouldEqual, ActionTrailingStop)
			So(manager.Do.Offset, ShouldEqual, 0.01)
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
