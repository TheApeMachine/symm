package tokenizer

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewMDL(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When NewMDL is called", func() {
			mdl := NewMDL()

			Convey("MinSegmentSymbols defaults to 2", func() {
				So(mdl, ShouldNotBeNil)
				So(mdl.MinSegmentSymbols, ShouldEqual, 2)
				So(mdl.LastGain(), ShouldEqual, 0)
			})
		})
	})
}

func TestMDLAnalyze(t *testing.T) {
	Convey("Setup", t, func() {
		mdl := NewMDL()
		mdl.MinSegmentSymbols = 2

		Convey("When symbol is empty", func() {
			cut, gain := mdl.Analyze("")

			Convey("Analyze returns no cut", func() {
				So(cut, ShouldBeFalse)
				So(gain, ShouldEqual, 0)
			})
		})

		Convey("When the stream crosses a bimodal boundary", func() {
			var (
				cutSeen bool
				gain    float64
			)

			for _, sym := range []string{
				"R00.R00", "R00.R01", "R01.R00", "R01.R01",
				"R02.R00", "R02.R01", "R03.R00", "R03.R01",
			} {
				cut, g := mdl.Analyze(sym)

				if cut {
					cutSeen = true
					gain = g
					break
				}
			}

			Convey("Analyze commits a cut", func() {
				So(cutSeen, ShouldBeTrue)
				So(gain, ShouldEqual, 4.1588830833596715)
				So(mdl.LastGain(), ShouldEqual, 1)
			})
		})
	})
}

func TestLastGain(t *testing.T) {
	Convey("Setup", t, func() {
		mdl := NewMDL()
		mdl.MinSegmentSymbols = 2
		mdl.lastGain = 1.5

		Convey("When lastGain exceeds 1", func() {
			Convey("LastGain is capped at 1", func() {
				So(mdl.LastGain(), ShouldEqual, 1)
			})
		})
	})
}
