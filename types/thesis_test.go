package types

import (
	"sync"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

/*
TestThesisAppendMeasurementsConcurrent proves independent signal actors can
replace the same source-symbol row in the shared Thesis without corrupting the
direct measurement map. The Thesis stores current evidence, not an append-only
history, so one row survives for this identity.
*/
func TestThesisAppendMeasurementsConcurrent(t *testing.T) {
	Convey("Given concurrent signal publications", t, func() {
		thesis := NewThesis(nil)
		var wg sync.WaitGroup

		Convey("When many goroutines publish the same source-symbol row", func() {
			for index := range 128 {
				wg.Go(func() {
					thesis.Measurements.Store(SourceCVD, []*Measurement{{
						Source: SourceCVD,
						Symbol: "BTC/USD",
						At:     time.Unix(int64(index), 0).UTC(),
					}})
				})
			}

			wg.Wait()
			count := 0
			thesis.Measurements.Range(func(_, value any) bool {
				for _, m := range value.([]*Measurement) {
					if m != nil {
						count++
					}
				}
				return true
			})

			Convey("Then the current row is retained without map corruption", func() {
				So(count, ShouldEqual, 1)
			})
		})
	})
}

func TestNoteLifecycle(t *testing.T) {
	Convey("Given a thesis phase transition", t, func() {
		thesis := NewThesis(nil)
		at := time.Unix(1, 0).UTC()
		thesis.NoteLifecycle("BTC/USD", LifecycleEntered, at)

		Convey("It should store the phase without a parallel journal", func() {
			phase, ok := thesis.Lifecycle.Load("BTC/USD")
			So(ok, ShouldBeTrue)
			So(phase, ShouldEqual, LifecycleEntered)
		})
	})
}

func BenchmarkNoteLifecycle(b *testing.B) {
	thesis := NewThesis(nil)
	at := time.Unix(1, 0).UTC()

	b.ReportAllocs()

	for b.Loop() {
		thesis.NoteLifecycle("BTC/USD", LifecycleManaging, at)
	}
}

/*
BenchmarkMeasurements measures the locked shared-Thesis signal publish path.
*/
func BenchmarkMeasurements(b *testing.B) {
	thesis := NewThesis(nil)
	row := &Measurement{Source: SourceCVD, Symbol: "BTC/USD", At: time.Unix(1, 0)}

	b.ReportAllocs()

	for b.Loop() {
		thesis.Measurements.Store(SourceCVD, []*Measurement{row})
	}
}
