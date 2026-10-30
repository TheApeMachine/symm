package tokenizer

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSequencerStep(t *testing.T) {
	Convey("Given a sequencer", t, func() {
		seq := NewSequencer()

		Convey("State survives Restore", func() {
			_ = seq.Step("R06.R00", 0.4)
			saved := seq.State()
			restored := NewSequencer()
			restored.Restore(saved)

			next := restored.Step("R07.R01", 0.5)
			So(next.Last().String(), ShouldNotBeBlank)
			So(len(restored.State().Root), ShouldBeGreaterThan, 0)
		})

		Convey("duplicate symbol refreshes cut brightness without growing grammar", func() {
			first := seq.Step("R06.R00", 0.3)
			second := seq.Step("R06.R00", 0.5)

			So(first.String(), ShouldEqual, "R06.R00")
			So(second.Len(), ShouldBeGreaterThan, 0)
			So(second.Last().Brightness, ShouldEqual, 0.5)
		})

		Convey("repeat symbol does not append to grammar", func() {
			_ = seq.Step("R06.R00", 0.3)
			before := len(seq.Grammar().RootSequence())
			_ = seq.Step("R06.R00", 0.3)
			So(len(seq.Grammar().RootSequence()), ShouldEqual, before)
		})

		Convey("oscillating symbols compress via sequitur", func() {
			for range 6 {
				_ = seq.Step("R07.R03", 0.5)
				_ = seq.Step("R08.R04", 0.5)
			}

			So(len(seq.Grammar().Tail(8)), ShouldBeLessThanOrEqualTo, 4)
		})
	})
}
