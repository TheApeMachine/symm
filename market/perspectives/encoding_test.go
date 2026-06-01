package perspectives

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"go.yaml.in/yaml/v3"
)

func TestUnitTypeUnmarshalYAML(t *testing.T) {
	convey.Convey("Given a branch encoded with enum names", t, func() {
		raw := []byte(`
category: laminar
observation: not_holding
regime: bullish
condition: ">="
unit: snr
action:
  type: limit
`)
		branch := Branch{}

		err := yaml.Unmarshal(raw, &branch)

		convey.Convey("It should decode the branch fields", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(branch.Category, convey.ShouldEqual, CategoryLaminar)
			convey.So(branch.Observation, convey.ShouldEqual, ObservationNotHolding)
			convey.So(branch.Regime, convey.ShouldEqual, RegimeBullish)
			convey.So(branch.Condition, convey.ShouldEqual, ConditionIsGreaterThanOrEqual)
			convey.So(branch.Unit, convey.ShouldEqual, UnitSNR)
			convey.So(branch.Action.Type, convey.ShouldEqual, ActionLimit)
		})
	})
}

func TestUnitTypeMarshalJSON(t *testing.T) {
	convey.Convey("Given enum values", t, func() {
		raw, err := UnitSNR.MarshalJSON()

		convey.Convey("It should encode readable names", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(string(raw), convey.ShouldEqual, `"snr"`)
		})
	})
}

func TestUnitTypeUnmarshalJSON(t *testing.T) {
	convey.Convey("Given a JSON enum name", t, func() {
		unit := UnitType(0)

		err := unit.UnmarshalJSON([]byte(`"snr"`))

		convey.Convey("It should decode the enum value", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(unit, convey.ShouldEqual, UnitSNR)
		})
	})

	convey.Convey("Given an unknown numeric enum value", t, func() {
		unit := UnitType(0)

		err := unit.UnmarshalJSON([]byte(`99`))

		convey.Convey("It should reject the value", func() {
			convey.So(err, convey.ShouldNotBeNil)
		})
	})
}

func TestUnitTypeMarshalYAML(t *testing.T) {
	convey.Convey("Given enum values", t, func() {
		branch := Branch{
			Observation: ObservationHolding,
			Regime:      RegimeBearish,
			Condition:   ConditionIsLessThan,
			Unit:        UnitConfidence,
			Action:      Action{Type: ActionSettlePosition},
		}

		raw, err := yaml.Marshal(branch)

		convey.Convey("It should encode readable names", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(string(raw), convey.ShouldContainSubstring, "observation: holding")
			convey.So(string(raw), convey.ShouldContainSubstring, "regime: bearish")
			convey.So(string(raw), convey.ShouldContainSubstring, "condition: <")
			convey.So(string(raw), convey.ShouldContainSubstring, "unit: confidence")
			convey.So(string(raw), convey.ShouldContainSubstring, "type: settle_position")
		})
	})
}
