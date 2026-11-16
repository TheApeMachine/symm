package strategy

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/runtime"
)

func TestOracleSides(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When lit veto and approve regions are present", func() {
			sides := oracleSides([]*runtime.Region{
				{Key: 9, Brightness: 1},
				{Key: 12, Brightness: 1},
				{Key: 18, Brightness: 0.4},
			})

			Convey("Each pair is named with side and brightness", func() {
				So(sides["vol"].(map[string]any)["side"], ShouldEqual, "veto")
				So(sides["day"].(map[string]any)["side"], ShouldEqual, "approve")
				So(sides["brd"].(map[string]any)["brightness"], ShouldEqual, 0.4)
			})
		})

		Convey("When regions are empty", func() {
			So(oracleSides(nil), ShouldBeNil)
		})
	})
}

func TestOracleGateID(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When oracle keys are mapped", func() {
			So(oracleGateID(9), ShouldEqual, "vol")
			So(oracleGateID(16), ShouldEqual, "bok")
			So(oracleGateID(24), ShouldEqual, "pk")
			So(oracleGateID(0), ShouldEqual, "")
		})
	})
}

func BenchmarkOracleSides(b *testing.B) {
	regions := []*runtime.Region{
		{Key: 10, Brightness: 1},
		{Key: 11, Brightness: 1},
		{Key: 14, Brightness: 1},
		{Key: 16, Brightness: 1},
		{Key: 17, Brightness: 0.2},
		{Key: 20, Brightness: 0.3},
		{Key: 22, Brightness: 0.1},
		{Key: 24, Brightness: 1},
	}

	b.ReportAllocs()

	for b.Loop() {
		_ = oracleSides(regions)
	}
}
