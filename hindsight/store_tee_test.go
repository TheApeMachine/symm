package hindsight

import (
	goruntime "runtime"
	"sync"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
)

func TestStoreTee_Push(t *testing.T) {
	Convey("One publisher and one drain preserve observations across ring wraps", t, func() {
		const capacity, observations = 16, 1024
		tee := NewStoreTee(t.Context(), "store-stream", capacity)
		defer func() { So(tee.Close(), ShouldBeNil) }()
		tee.Transition(runtime.READY)
		finished := make(chan struct{})

		go func() {
			defer close(finished)

			for sequence := 1; sequence <= observations; sequence++ {
				for tee.Pending() == capacity {
					goruntime.Gosched()
				}

				tee.Push(&data.Measurement[float64]{SeqIdx: int64(sequence)})
			}
		}()

		for sequence := 1; sequence <= observations; {
			pointer := tee.Next()

			if pointer == nil {
				goruntime.Gosched()
				continue
			}

			So((*data.Measurement[float64])(pointer).SeqIdx, ShouldEqual, sequence)
			sequence++
		}

		<-finished
		So(tee.Pending(), ShouldEqual, 0)
	})
	Convey("Concurrent consumers retain each accepted publication in producer order", t, func() {
		const producers, observations = 8, 64
		tee := NewStoreTee(t.Context(), "concurrent-store", producers*observations)
		defer func() { So(tee.Close(), ShouldBeNil) }()
		tee.Transition(runtime.READY)
		var workers sync.WaitGroup

		for producer := 0; producer < producers; producer++ {
			workers.Go(func() {
				for sequence := 1; sequence <= observations; sequence++ {
					tee.Push(&data.Measurement[float64]{ID: producer, SeqIdx: int64(sequence)})
				}
			})
		}

		workers.Wait()
		sequences := make([]int64, producers)

		for received := 0; received < producers*observations; received++ {
			measurement := (*data.Measurement[float64])(tee.Next())
			So(measurement, ShouldNotBeNil)
			sequences[measurement.ID]++
			So(measurement.SeqIdx, ShouldEqual, sequences[measurement.ID])
		}

		So(tee.Pending(), ShouldEqual, 0)
	})

}

func TestStoreTee_Next(t *testing.T) {
	Convey("A storage tee stays empty until explicitly activated", t, func() {
		tee := NewStoreTee(t.Context(), "store-test", 4)
		defer func() { So(tee.Close(), ShouldBeNil) }()
		So(tee.Status(), ShouldEqual, runtime.INIT)
		So(tee.Next() == nil, ShouldBeTrue)
		measurement := &data.Measurement[float64]{Label: "BTC/USD", SeqIdx: 1}
		tee.Push(measurement)
		tee.Transition(runtime.READY)
		So(tee.Next() == nil, ShouldBeTrue)

		Convey("Reusing a producer preserves each queued observation", func() {
			measurement.SeqIdx = 1
			tee.Push(measurement)
			measurement.SeqIdx = 2
			tee.Push(measurement)
			So((*data.Measurement[float64])(tee.Next()).SeqIdx, ShouldEqual, 1)
			So((*data.Measurement[float64])(tee.Next()).SeqIdx, ShouldEqual, 2)
		})

		Convey("Successive batches preserve owned observations", func() {
			for batch := 0; batch < 3; batch++ {
				first := data.NewMeasurement[float64]("feed", nil)
				first.Label, first.SeqIdx = "BTC/USD", int64(batch*2+1)
				second := data.NewMeasurement[float64]("feed", nil)
				second.Label, second.SeqIdx = "ETH/USD", first.SeqIdx+1
				tee.Push(first)
				tee.Push(second)
				So((*data.Measurement[float64])(tee.Next()), ShouldResemble, first)
				So((*data.Measurement[float64])(tee.Next()), ShouldResemble, second)
				So(tee.Next() == nil, ShouldBeTrue)
				So(tee.Error(), ShouldBeNil)
			}
		})
	})
}

func BenchmarkStoreTee_Next(b *testing.B) {
	tee := NewStoreTee(b.Context(), "store-benchmark", 4)
	tee.Transition(runtime.READY)
	measurement := &data.Measurement[float64]{Label: "BTC/USD", SeqIdx: 1}
	b.ResetTimer()

	for iteration := 0; iteration < b.N; iteration++ {
		tee.Push(measurement)

		if (*data.Measurement[float64])(tee.Next()).SeqIdx != measurement.SeqIdx {
			b.Fatal("observation changed")
		}
	}

	b.StopTimer()

	if err := tee.Close(); err != nil {
		b.Fatal(err)
	}
}
