package hindsight

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
)

func TestStoreTee_Next(t *testing.T) {
	Convey("A storage tee stays empty until explicitly activated", t, func() {
		tee := NewStoreTee("store-test", 4)
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
				first := &data.Measurement[float64]{Label: "BTC/USD", SeqIdx: int64(batch*2 + 1)}
				second := &data.Measurement[float64]{Label: "ETH/USD", SeqIdx: first.SeqIdx + 1}
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
	tee := NewStoreTee("store-benchmark", 4)
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
