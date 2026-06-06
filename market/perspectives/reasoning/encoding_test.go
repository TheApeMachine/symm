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

		Convey("It parses five exit managers and five spot entry strategies", func() {
			So(len(thoughts), ShouldEqual, 10)
		})

		Convey("Exit managers precede entries and the scalp manager is tightest", func() {
			scalpExit := thoughts[0]
			So(scalpExit.When.All[0].Lifecycle, ShouldEqual, types.ObservationHolding)
			So(scalpExit.When.All[1].Category, ShouldEqual, types.CategoryVerticalIgnition)
			So(scalpExit.Then[0].Do.Type, ShouldEqual, ActionSettlePosition)
		})

		Convey("The flash-pump entry confirms ignition level and price follow-through before limit", func() {
			scalpEntry := thoughts[5]
			confirm := scalpEntry.Then[0]
			So(confirm.Do.Type, ShouldEqual, ActionLimit)
			So(confirm.When.All[0].Lifecycle, ShouldEqual, types.ObservationNotHolding)
			So(confirm.When.All[1].Op, ShouldEqual, ComparisonAtLeast)
			So(confirm.When.All[1].Category, ShouldEqual, types.CategoryVerticalIgnition)
			So(confirm.When.All[3].Subject, ShouldEqual, SubjectPrice)
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

		Convey("Every spot entry denies dead and choppy regimes", func() {
			for index := 5; index < len(thoughts); index++ {
				entry := thoughts[index]
				hasDeadDeny := false
				hasChoppyDeny := false

				for _, operand := range entry.When.All {
					if operand.Not == nil {
						continue
					}

					if operand.Not.Subject == SubjectRegime && operand.Not.Regime == types.RegimeDead {
						hasDeadDeny = true
					}

					if operand.Not.Subject == SubjectRegime && operand.Not.Regime == types.RegimeChoppy {
						hasChoppyDeny = true
					}
				}

				So(hasDeadDeny, ShouldBeTrue)
				So(hasChoppyDeny, ShouldBeTrue)
			}
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
