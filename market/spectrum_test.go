package market

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/logic"
)

func TestSymbolStateAbsorb(t *testing.T) {
	Convey("Given a symbol spectrum accumulator", t, func() {
		state := newSymbolState(32)

		Convey("It should overwrite duplicate source updates before the spectrum completes", func() {
			first := logic.Measurement{
				Source:     logic.SourceHawkes,
				Symbol:     "BTC/USD",
				Price:      1,
				Strength:   1,
				Volume:     1,
				Spread:     1,
				Elapsed:    1,
				Confidence: 1,
				Surprise:   1,
				ObservedAt: time.Now(),
			}

			complete, absorbErr := state.absorb(first)

			So(absorbErr, ShouldBeNil)
			So(complete, ShouldBeFalse)

			second := first
			second.Strength = 2

			complete, absorbErr = state.absorb(second)

			So(absorbErr, ShouldBeNil)
			So(complete, ShouldBeFalse)

			index, indexErr := logic.SourceIndex(logic.SourceHawkes)

			So(indexErr, ShouldBeNil)
			So(state.slots[index].Strength, ShouldEqual, 2)
		})

		Convey("It should complete only after all spectrum sources report", func() {
			for sourceIndex, source := range logic.SpectrumSources {
				measurement := logic.Measurement{
					Source:     source,
					Symbol:     "BTC/USD",
					Price:      1,
					Strength:   1,
					Volume:     1,
					Spread:     1,
					Elapsed:    1,
					Confidence: 1,
					Surprise:   1,
					ObservedAt: time.Now(),
				}

				complete, absorbErr := state.absorb(measurement)

				if sourceIndex < logic.SourceCount-1 {
					So(absorbErr, ShouldBeNil)
					So(complete, ShouldBeFalse)

					continue
				}

				So(absorbErr, ShouldBeNil)
				So(complete, ShouldBeTrue)
			}
		})
	})
}
