package reasoning

import (
	"os"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func TestParseThoughtsPlaybook(t *testing.T) {
	Convey("Given the production playbook", t, func() {
		raw, err := os.ReadFile("cfg/perspectives.yaml")
		So(err, ShouldBeNil)

		thoughts, err := ParseThoughts(raw)
		So(err, ShouldBeNil)

		Convey("It parses five exit managers and seven entry strategies", func() {
			So(len(thoughts), ShouldEqual, 12)
		})

		Convey("Exit managers precede entries and the scalp manager is tightest", func() {
			scalpExit := thoughts[0]
			So(scalpExit.When.All[0].Lifecycle, ShouldEqual, types.ObservationHolding)
			So(scalpExit.When.All[1].Category, ShouldEqual, types.CategoryVerticalIgnition)
			So(scalpExit.Then[0].Do.Type, ShouldEqual, ActionSettlePosition)
		})

		Convey("The flash-pump entry confirms an ignition edge before iceberg", func() {
			scalpEntry := thoughts[5]
			confirm := scalpEntry.Then[0]
			So(confirm.Do.Type, ShouldEqual, ActionIceberg)
			So(confirm.When.All[0].Lifecycle, ShouldEqual, types.ObservationNotHolding)
			So(confirm.When.All[1].Op, ShouldEqual, ComparisonCrossedUp)
		})

		Convey("The momentum entry requires rising quote volume at confirmation", func() {
			trendEntry := thoughts[6]
			confirm := trendEntry.Then[0]
			So(confirm.Do.Type, ShouldEqual, ActionLimit)

			hasVolume := false

			for _, operand := range confirm.When.All {
				if operand.Subject == SubjectVolume {
					hasVolume = true
				}
			}

			So(hasVolume, ShouldBeTrue)
		})

		Convey("Bearish and herd-fade entries sell to open", func() {
			breakdown := thoughts[10]
			fade := thoughts[11]
			So(breakdown.Then[0].Do.Side, ShouldEqual, trading.Sell)
			So(fade.Then[0].Do.Side, ShouldEqual, trading.Sell)
		})

		Convey("The universal fallback manager is the last branch", func() {
			fallback := thoughts[4]
			So(fallback.When.Lifecycle, ShouldEqual, types.ObservationHolding)
			So(fallback.Then[0].Do.Type, ShouldEqual, ActionSettlePosition)
		})
	})
}

func TestMarshalThoughtsRoundTrips(t *testing.T) {
	Convey("Given the production playbook", t, func() {
		raw, err := os.ReadFile("cfg/perspectives.yaml")
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
