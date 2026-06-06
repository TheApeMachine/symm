package perspectives

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives/reasoning"
	"github.com/theapemachine/symm/market/perspectives/types"
	"go.yaml.in/yaml/v3"
)

func TestUnitTypeUnmarshalYAML(t *testing.T) {
	convey.Convey("Given a predicate encoded with enum names", t, func() {
		raw := []byte(`
subject: signal
category: laminar
regime: bullish
op: at_least
unit: snr
`)
		predicate := Predicate{}

		err := yaml.Unmarshal(raw, &predicate)

		convey.Convey("It should decode the enum-named fields", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(predicate.Subject, convey.ShouldEqual, SubjectSignal)
			convey.So(predicate.Category, convey.ShouldEqual, types.CategoryLaminar)
			convey.So(predicate.Regime, convey.ShouldEqual, types.RegimeBullish)
			convey.So(predicate.Op, convey.ShouldEqual, ComparisonAtLeast)
			convey.So(predicate.Unit, convey.ShouldEqual, reasoning.UnitSNR)
		})
	})
}

func TestUnitTypeMarshalJSON(t *testing.T) {
	convey.Convey("Given enum values", t, func() {
		raw, err := reasoning.UnitSNR.MarshalJSON()

		convey.Convey("It should encode readable names", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(string(raw), convey.ShouldEqual, `"snr"`)
		})
	})
}

func TestUnitTypeUnmarshalJSON(t *testing.T) {
	convey.Convey("Given a JSON enum name", t, func() {
		unit := reasoning.UnitType(0)

		err := unit.UnmarshalJSON([]byte(`"snr"`))

		convey.Convey("It should decode the enum value", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(unit, convey.ShouldEqual, reasoning.UnitSNR)
		})
	})

	convey.Convey("Given an unknown numeric enum value", t, func() {
		unit := reasoning.UnitType(0)

		err := unit.UnmarshalJSON([]byte(`99`))

		convey.Convey("It should reject the value", func() {
			convey.So(err, convey.ShouldNotBeNil)
		})
	})
}

func TestUnitTypeMarshalYAML(t *testing.T) {
	convey.Convey("Given enum values", t, func() {
		predicate := reasoning.Predicate{
			Subject: reasoning.SubjectRegime,
			Regime:  types.RegimeBearish,
			Op:      reasoning.ComparisonBelow,
			Unit:    reasoning.UnitConfidence,
		}

		raw, err := yaml.Marshal(predicate)

		convey.Convey("It should encode readable names", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(string(raw), convey.ShouldContainSubstring, "subject: regime")
			convey.So(string(raw), convey.ShouldContainSubstring, "regime: bearish")
			convey.So(string(raw), convey.ShouldContainSubstring, "op: below")
			convey.So(string(raw), convey.ShouldContainSubstring, "unit: confidence")
		})
	})
}
