package tokenizer

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSequencerStep(t *testing.T) {
	Convey("Given a sequencer", t, func() {
		seq := NewSequencer()

		Convey("State survives Restore", func() {
			_, _ = seq.Step("R02", 0.4)
			saved := seq.State()
			restored := NewSequencer()
			restored.Restore(saved)

			next, mutated := restored.Step("R00", 0.5)
			So(next.Last().String(), ShouldNotBeBlank)
			So(mutated, ShouldBeTrue)
			So(len(restored.State().Root), ShouldBeGreaterThan, 0)
		})

		Convey("duplicate regime refreshes cut brightness without growing grammar", func() {
			first, firstMutated := seq.Step("R00", 0.3)
			second, secondMutated := seq.Step("R00", 0.5)

			So(first.String(), ShouldEqual, "R00")
			So(firstMutated, ShouldBeTrue)
			So(second.Len(), ShouldBeGreaterThan, 0)
			So(second.Last().Brightness, ShouldEqual, 0.5)
			So(secondMutated, ShouldBeFalse)
		})

		Convey("repeat regime does not append to grammar", func() {
			_, _ = seq.Step("R00", 0.3)
			before := len(seq.grammar.RootSequence())
			_, mutated := seq.Step("R00", 0.3)
			So(len(seq.grammar.RootSequence()), ShouldEqual, before)
			So(mutated, ShouldBeFalse)
		})

		Convey("Step builds a multi-tick precursor sequence", func() {
			_, _ = seq.Step("R02", 0.5)
			_, _ = seq.Step("R00", 0.7)
			cut, mutated := seq.Step("R06", 0.8)
			So(cut.String(), ShouldEqual, "R02/R00/R06")
			So(cut.Len(), ShouldEqual, 3)
			So(mutated, ShouldBeTrue)
		})

		Convey("oscillating symbols compress via sequitur", func() {
			for range 6 {
				_, _ = seq.Step("R07", 0.5)
				_, _ = seq.Step("R08", 0.5)
			}

			So(len(seq.grammar.Tail(8)), ShouldBeLessThanOrEqualTo, 4)
		})
	})
}
