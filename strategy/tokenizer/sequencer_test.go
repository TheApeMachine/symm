package tokenizer

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewSequencer(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When NewSequencer is called", func() {
			seq := NewSequencer()

			Convey("Sequencer starts with empty grammar", func() {
				So(seq, ShouldNotBeNil)
				So(len(seq.State().Root), ShouldEqual, 0)
			})
		})
	})
}

func TestReset(t *testing.T) {
	Convey("Setup", t, func() {
		seq := NewSequencer()

		Convey("When Reset follows a stepped sentence", func() {
			_, _ = seq.Step("R01", 0.5)
			So(seq.State().LastEmitted, ShouldNotEqual, "")

			seq.Reset()

			Convey("lastEmitted clears while rules remain", func() {
				So(seq.State().LastEmitted, ShouldEqual, "")
				So(len(seq.State().Root), ShouldEqual, 0)
			})
		})
	})
}

func TestStep(t *testing.T) {
	Convey("Setup", t, func() {
		seq := NewSequencer()

		Convey("When duplicate regime refreshes brightness", func() {
			first, firstMutated := seq.Step("R00", 0.3)
			second, secondMutated := seq.Step("R00", 0.5)

			Convey("Cut string stays R00 with tail brightness 0.5", func() {
				So(first.String(), ShouldEqual, "R00")
				So(firstMutated, ShouldBeTrue)
				So(second.Last().Brightness, ShouldEqual, 0.5)
				So(secondMutated, ShouldBeFalse)
			})
		})

		Convey("When three regimes step in sequence", func() {
			_, _ = seq.Step("R02", 0.5)
			_, _ = seq.Step("R00", 0.7)
			cut, mutated := seq.Step("R06", 0.8)

			Convey("Cut is R02/R00/R06", func() {
				So(cut.String(), ShouldEqual, "R02/R00/R06")
				So(cut.Len(), ShouldEqual, 3)
				So(mutated, ShouldBeTrue)
			})
		})
	})
}

func TestStepPath(t *testing.T) {
	Convey("Setup", t, func() {
		seq := NewSequencer()

		Convey("When StepPath receives empty symbols", func() {
			cut, mutated := seq.StepPath([]string{}, 0.5)

			Convey("Cut is empty", func() {
				So(cut.Len(), ShouldEqual, 0)
				So(mutated, ShouldBeFalse)
			})
		})
	})
}

func TestState(t *testing.T) {
	Convey("Setup", t, func() {
		seq := NewSequencer()

		Convey("When State is called after Step", func() {
			_, _ = seq.Step("R02", 0.4)
			state := seq.State()

			Convey("LastEmitted matches the stepped key", func() {
				So(state.LastEmitted, ShouldEqual, "R02")
				So(len(state.Root), ShouldEqual, 1)
			})
		})
	})
}

func TestRestore(t *testing.T) {
	Convey("Setup", t, func() {
		seq := NewSequencer()
		_, _ = seq.Step("R02", 0.4)
		saved := seq.State()

		restored := NewSequencer()
		restored.Restore(saved)

		Convey("When Restore replays saved grammar", func() {
			next, mutated := restored.Step("R00", 0.5)

			Convey("Next cut extends the restored root", func() {
				So(mutated, ShouldBeTrue)
				So(next.String(), ShouldEqual, "R02/R00")
				So(len(restored.State().Root), ShouldEqual, 2)
			})
		})
	})
}

func BenchmarkNewSequencer(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		_ = NewSequencer()
	}
}

func BenchmarkReset(b *testing.B) {
	seq := NewSequencer()
	_, _ = seq.Step("R01", 0.5)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		seq.Reset()
		seq.Step("R01", 0.5)
	}
}

func BenchmarkStep(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		seq := NewSequencer()
		seq.Step("R02", 0.5)
		seq.Step("R00", 0.7)
		seq.Step("R06", 0.8)
	}
}

func BenchmarkStepPath(b *testing.B) {
	seq := NewSequencer()

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		seq.StepPath([]string{}, 0.5)
	}
}

func BenchmarkState(b *testing.B) {
	seq := NewSequencer()
	_, _ = seq.Step("R02", 0.4)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = seq.State()
	}
}

func BenchmarkRestore(b *testing.B) {
	saved := func() SequencerState {
		seq := NewSequencer()
		_, _ = seq.Step("R02", 0.4)
		return seq.State()
	}()

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		seq := NewSequencer()
		seq.Restore(saved)
	}
}
