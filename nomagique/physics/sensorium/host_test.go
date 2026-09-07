package sensorium

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

/*
batchOf builds a State carrying one particle per supplied content identity, at a
position that encodes which batch it came from so a merge can be checked for
having preserved or replaced a row.
*/
func batchOf(marker float32, contentIDs ...int64) *State {
	batch := newState(len(contentIDs))

	for index, contentID := range contentIDs {
		batch.ContentIDs[index] = contentID
		batch.TokenIDs[index] = contentID
		batch.Energy[index] = marker
		batch.Mass[index] = 1
		batch.Pos[index*3+0] = marker
	}

	return batch
}

func TestMerge(t *testing.T) {
	Convey("Given a resident domain holding a book snapshot", t, func() {
		manifold := &Manifold{}
		manifold.merge(batchOf(1, 10, 11, 12))

		So(manifold.state.N, ShouldEqual, 3)

		Convey("An incremental update of one known order keeps the population", func() {
			manifold.merge(batchOf(2, 11))

			So(manifold.state.N, ShouldEqual, 3)

			Convey("And refreshes only the observed order", func() {
				So(manifold.state.Energy[1], ShouldEqual, 2)
				So(manifold.state.Energy[0], ShouldEqual, 1)
				So(manifold.state.Energy[2], ShouldEqual, 1)
			})

			Convey("Without restarting its integrated trajectory", func() {
				So(manifold.state.Pos[3], ShouldEqual, 1)
			})
		})

		Convey("An order never seen before is appended", func() {
			manifold.merge(batchOf(3, 13))

			So(manifold.state.N, ShouldEqual, 4)
			So(manifold.state.ContentIDs[3], ShouldEqual, 13)
			So(manifold.state.Energy[3], ShouldEqual, 3)
		})

		Convey("A batch mixing known and new orders grows by the new ones only", func() {
			manifold.merge(batchOf(4, 10, 14, 15))

			So(manifold.state.N, ShouldEqual, 5)
			So(manifold.state.Energy[0], ShouldEqual, 4)
		})

		Convey("An order absent from a batch is not evicted", func() {
			manifold.merge(batchOf(5, 10))

			So(manifold.state.N, ShouldEqual, 3)
			So(manifold.state.ContentIDs[2], ShouldEqual, 12)
		})
	})
}

func TestRemove(t *testing.T) {
	Convey("Given a resident domain with three identified particles", t, func() {
		manifold := &Manifold{}
		manifold.merge(batchOf(1, 10, 11, 12))

		Convey("An explicit departure evicts only the named particle", func() {
			remaining, err := manifold.Remove([]int64{11})

			So(err, ShouldBeNil)
			So(remaining, ShouldEqual, 2)
			So(manifold.state.ContentIDs, ShouldResemble, []int64{10, 12})
			So(manifold.resident[10], ShouldEqual, 0)
			So(manifold.resident[12], ShouldEqual, 1)
		})

		Convey("An unknown departure fails without mutating the population", func() {
			remaining, err := manifold.Remove([]int64{99})

			So(err, ShouldNotBeNil)
			So(remaining, ShouldEqual, 3)
			So(manifold.state.ContentIDs, ShouldResemble, []int64{10, 11, 12})
		})

		Convey("A duplicate departure fails without partially mutating the population", func() {
			remaining, err := manifold.Remove([]int64{11, 11})

			So(err, ShouldNotBeNil)
			So(remaining, ShouldEqual, 3)
			So(manifold.state.ContentIDs, ShouldResemble, []int64{10, 11, 12})
		})
	})
}

func TestManifoldSpectralPeaks(t *testing.T) {
	Convey("Given synchronized resident complex modes", t, func() {
		fluid, err := newWorkspace(8, 8, 8)
		So(err, ShouldBeNil)
		Reset(func() { fluid.Close() })
		manifold := &Manifold{work: fluid}
		real := fluid.psiModeReal.Float32Slice()
		imag := fluid.psiModeImag.Float32Slice()
		clear(real)
		clear(imag)
		So(manifold.SpectralPeaks(), ShouldBeEmpty)
		real[0], real[2], real[3], imag[5] = 10, 2, 2, 3
		peaks := manifold.SpectralPeaks()
		So(peaks, ShouldResemble, []SpectralPeak{
			{Index: 2, Frequency: fluid.omegaLattice.Float32Slice()[2], Power: 4},
			{Index: 5, Frequency: fluid.omegaLattice.Float32Slice()[5], Power: 9},
		})

		Convey("An evolving spectrum removes old peaks and retains weak local maxima", func() {
			clear(real)
			clear(imag)
			imag[4] = 0.0001
			updated := manifold.SpectralPeaks()
			So(updated, ShouldHaveLength, 1)
			So(updated[0].Index, ShouldEqual, 4)
			So(updated[0].Power, ShouldBeGreaterThan, 0)
			So(peaks[0].Power, ShouldEqual, 4)
		})
	})
}

func BenchmarkRemove(b *testing.B) {
	contentIDs := make([]int64, 1024)

	for index := range contentIDs {
		contentIDs[index] = int64(index + 1)
	}

	manifold := &Manifold{}
	manifold.merge(batchOf(1, contentIDs...))
	returning := batchOf(2, contentIDs[0])
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := manifold.Remove(returning.ContentIDs); err != nil {
			b.Fatal(err)
		}

		manifold.merge(returning)
	}
}

func BenchmarkManifoldSpectralPeaks(b *testing.B) {
	fluid, err := newWorkspace(8, 8, 8)

	if err != nil {
		b.Fatal(err)
	}

	defer fluid.Close()
	manifold := &Manifold{work: fluid}
	fluid.psiModeReal.Float32Slice()[2] = 1
	b.ReportAllocs()

	for b.Loop() {
		manifold.SpectralPeaks()
	}
}
