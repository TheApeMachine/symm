package tokenizer

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMDLState(t *testing.T) {
	Convey("Setup", t, func() {
		mdl := NewMDL()
		mdl.stream = []uint32{1, 2}
		mdl.offset = 1
		mdl.vocab["R00"] = 1
		mdl.vocab["R01"] = 2
		mdl.nextID = 3
		mdl.dist.add(1)
		mdl.dist.add(2)
		mdl.dist.add(1)
		mdl.lastGain = 0.42

		Convey("When State snapshots the segmenter", func() {
			state := mdl.State()

			Convey("Exported fields match live memory", func() {
				So(state.Stream, ShouldResemble, []uint32{1, 2})
				So(state.Offset, ShouldEqual, 1)
				So(state.Vocab["R00"], ShouldEqual, uint32(1))
				So(state.Vocab["R01"], ShouldEqual, uint32(2))
				So(state.NextID, ShouldEqual, uint32(3))
				So(state.Dist.Counts[1], ShouldEqual, 2)
				So(state.Dist.Counts[2], ShouldEqual, 1)
				So(state.Dist.Total, ShouldEqual, 3)
				So(state.Dist.Distinct, ShouldEqual, 2)
				So(state.LastGain, ShouldEqual, 0.42)
			})

			Convey("Mutating the snapshot does not alias live maps or stream", func() {
				state.Stream[0] = 99
				state.Vocab["R00"] = 99
				state.Dist.Counts[1] = 99

				So(mdl.stream[0], ShouldEqual, uint32(1))
				So(mdl.vocab["R00"], ShouldEqual, uint32(1))
				So(mdl.dist.counts[1], ShouldEqual, 2)
			})
		})
	})
}

func TestMDLRestore(t *testing.T) {
	Convey("Setup", t, func() {
		mdl := NewMDL()
		mdl.Analyze("R00.R00")

		Convey("When Restore replays a full snapshot", func() {
			state := MDLState{
				Stream: []uint32{7, 8},
				Offset: 1,
				Vocab:  map[string]uint32{"R00.R00": 7, "R01.R00": 8},
				NextID: 9,
				Dist: symbolHistogramState{
					Counts:   map[uint32]int{7: 2, 8: 1},
					Total:    3,
					Distinct: 2,
				},
				LastGain: 0.9,
			}
			mdl.Restore(state)

			Convey("Live fields match the snapshot", func() {
				So(mdl.stream, ShouldResemble, []uint32{7, 8})
				So(mdl.offset, ShouldEqual, 1)
				So(mdl.vocab["R00.R00"], ShouldEqual, uint32(7))
				So(mdl.vocab["R01.R00"], ShouldEqual, uint32(8))
				So(mdl.nextID, ShouldEqual, uint32(9))
				So(mdl.dist.counts[7], ShouldEqual, 2)
				So(mdl.dist.counts[8], ShouldEqual, 1)
				So(mdl.dist.total, ShouldEqual, 3)
				So(mdl.dist.numDistinct, ShouldEqual, 2)
				So(mdl.lastGain, ShouldEqual, 0.9)
			})
		})

		Convey("When NextID is zero and Vocab is nil", func() {
			mdl.Restore(MDLState{
				Stream: []uint32{},
				NextID: 0,
				Vocab:  nil,
				Dist:   symbolHistogramState{Counts: map[uint32]int{}},
			})

			Convey("nextID becomes 1 and vocab is allocated", func() {
				So(mdl.nextID, ShouldEqual, uint32(1))
				So(mdl.vocab, ShouldNotBeNil)
				So(len(mdl.vocab), ShouldEqual, 0)
				So(len(mdl.stream), ShouldEqual, 0)
			})
		})

		Convey("When Restore clones stream and vocab", func() {
			state := MDLState{
				Stream: []uint32{1},
				Vocab:  map[string]uint32{"a": 1},
				NextID: 2,
				Dist:   symbolHistogramState{Counts: map[uint32]int{1: 1}, Total: 1, Distinct: 1},
			}
			mdl.Restore(state)
			state.Stream[0] = 99
			state.Vocab["a"] = 99

			Convey("Live memory is not aliased", func() {
				So(mdl.stream[0], ShouldEqual, uint32(1))
				So(mdl.vocab["a"], ShouldEqual, uint32(1))
			})
		})
	})
}

func TestCloneVocab(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When vocab is nil", func() {
			out := cloneVocab(nil)

			Convey("An empty map is returned", func() {
				So(out, ShouldNotBeNil)
				So(len(out), ShouldEqual, 0)
			})
		})

		Convey("When vocab is empty", func() {
			out := cloneVocab(map[string]uint32{})

			Convey("An empty map is returned", func() {
				So(out, ShouldNotBeNil)
				So(len(out), ShouldEqual, 0)
			})
		})

		Convey("When vocab has entries", func() {
			src := map[string]uint32{"R00": 1, "R01": 2}
			out := cloneVocab(src)
			src["R00"] = 99

			Convey("The copy is independent", func() {
				So(out["R00"], ShouldEqual, uint32(1))
				So(out["R01"], ShouldEqual, uint32(2))
				So(len(out), ShouldEqual, 2)
			})
		})
	})
}

func TestHistogramSnapshot(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When hist is nil", func() {
			state := histogramSnapshot(nil)

			Convey("Counts is an empty map with zero totals", func() {
				So(state.Counts, ShouldNotBeNil)
				So(len(state.Counts), ShouldEqual, 0)
				So(state.Total, ShouldEqual, 0)
				So(state.Distinct, ShouldEqual, 0)
			})
		})

		Convey("When hist has counts", func() {
			hist := newSymbolHistogram()
			hist.add(1)
			hist.add(1)
			hist.add(2)
			state := histogramSnapshot(hist)
			hist.counts[1] = 99

			Convey("Snapshot matches and does not alias", func() {
				So(state.Counts[1], ShouldEqual, 2)
				So(state.Counts[2], ShouldEqual, 1)
				So(state.Total, ShouldEqual, 3)
				So(state.Distinct, ShouldEqual, 2)
			})
		})
	})
}

func TestHistogramFrom(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When state carries counts", func() {
			hist := histogramFrom(symbolHistogramState{
				Counts:   map[uint32]int{3: 4, 5: 1},
				Total:    5,
				Distinct: 2,
			})

			Convey("The histogram is rebuilt exactly", func() {
				So(hist, ShouldNotBeNil)
				So(hist.counts[3], ShouldEqual, 4)
				So(hist.counts[5], ShouldEqual, 1)
				So(hist.total, ShouldEqual, 5)
				So(hist.numDistinct, ShouldEqual, 2)
			})
		})

		Convey("When Counts is nil", func() {
			hist := histogramFrom(symbolHistogramState{
				Counts:   nil,
				Total:    0,
				Distinct: 0,
			})

			Convey("An empty histogram is returned", func() {
				So(hist, ShouldNotBeNil)
				So(hist.counts, ShouldNotBeNil)
				So(len(hist.counts), ShouldEqual, 0)
				So(hist.total, ShouldEqual, 0)
				So(hist.numDistinct, ShouldEqual, 0)
			})
		})
	})
}

func BenchmarkMDLState(b *testing.B) {
	mdl := NewMDL()
	mdl.stream = []uint32{1, 2}
	mdl.offset = 1
	mdl.vocab["R00"] = 1
	mdl.vocab["R01"] = 2
	mdl.nextID = 3
	mdl.dist.add(1)
	mdl.dist.add(2)
	mdl.dist.add(1)
	mdl.lastGain = 0.42

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = mdl.State()
	}
}

func BenchmarkMDLRestore(b *testing.B) {
	mdl := NewMDL()
	state := MDLState{
		Stream: []uint32{7, 8},
		Offset: 1,
		Vocab:  map[string]uint32{"R00.R00": 7, "R01.R00": 8},
		NextID: 9,
		Dist: symbolHistogramState{
			Counts:   map[uint32]int{7: 2, 8: 1},
			Total:    3,
			Distinct: 2,
		},
		LastGain: 0.9,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		mdl.Restore(state)
	}
}

func BenchmarkCloneVocab(b *testing.B) {
	src := map[string]uint32{"R00": 1, "R01": 2}

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = cloneVocab(src)
	}
}

func BenchmarkHistogramSnapshot(b *testing.B) {
	hist := newSymbolHistogram()
	hist.add(1)
	hist.add(1)
	hist.add(2)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = histogramSnapshot(hist)
	}
}

func BenchmarkHistogramFrom(b *testing.B) {
	state := symbolHistogramState{
		Counts:   map[uint32]int{3: 4, 5: 1},
		Total:    5,
		Distinct: 2,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = histogramFrom(state)
	}
}
