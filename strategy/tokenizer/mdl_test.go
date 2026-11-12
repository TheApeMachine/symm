package tokenizer

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMDL(t *testing.T) {
	Convey("Given an MDL segmenter on grammar symbols", t, func() {
		mdl := NewMDL()
		mdl.MinSegmentSymbols = 2

		Convey("detectBoundary finds a split in a bimodal window", func() {
			symbols := []uint32{1, 1, 1, 1, 1, 2, 2, 2, 2, 2}
			dist := newSymbolHistogram()

			for _, id := range symbols {
				dist.add(id)
			}

			ok, k, gain := mdl.detectBoundary(symbols, dist)

			So(ok, ShouldBeTrue)
			So(k, ShouldEqual, 5)
			So(gain, ShouldBeGreaterThan, 0)
		})

		Convey("Analyze commits a cut and drops the consumed prefix", func() {
			ingested := 0
			cutSeen := false

			for _, sym := range []string{
				"R00.R00", "R00.R01", "R01.R00", "R01.R01",
				"R02.R00", "R02.R01", "R03.R00", "R03.R01",
			} {
				ingested++
				cut, _ := mdl.Analyze(sym)

				if !cut {
					continue
				}

				cutSeen = true
				So(mdl.offset, ShouldEqual, 0)
				So(len(mdl.stream), ShouldBeLessThan, ingested)
				break
			}

			So(cutSeen, ShouldBeTrue)
		})
	})
}
