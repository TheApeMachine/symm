package perspectives

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSourceTypeString(t *testing.T) {
	Convey("Given source types", t, func() {
		Convey("It should map to dashboard names", func() {
			So(SourceFluid.String(), ShouldEqual, "fluid")
			So(SourceHawkes.String(), ShouldEqual, "hawkes")
			So(SourceToxicity.String(), ShouldEqual, "toxicity")
		})

		Convey("It should return empty for SourceNone", func() {
			So(SourceNone.String(), ShouldBeBlank)
		})
	})
}

func TestMeasurementFields(t *testing.T) {
	Convey("Given a measurement row", t, func() {
		row := Measurement{
			Symbol:     "BTC/EUR",
			Source:     SourceFluid,
			Category:   CategoryLaminar,
			Strength:   0.8,
			Confidence: 0.6,
			SNR:        1.5,
			Last:       50_000,
			SpreadBPS:  12,
		}

		Convey("It should carry symbol-scoped signal fields", func() {
			So(row.Symbol, ShouldEqual, "BTC/EUR")
			So(row.Source, ShouldEqual, SourceFluid)
			So(row.SNR, ShouldBeGreaterThan, row.Confidence)
		})
	})
}
