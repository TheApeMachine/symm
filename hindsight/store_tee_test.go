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

		Convey("Successive batches preserve the actual measurement pointers", func() {
			for batch := 0; batch < 3; batch++ {
				first := &data.Measurement[float64]{Label: "BTC/USD", SeqIdx: int64(batch*2 + 1)}
				second := &data.Measurement[float64]{Label: "ETH/USD", SeqIdx: first.SeqIdx + 1}
				tee.Push(first)
				tee.Push(second)
				So((*data.Measurement[float64])(tee.Next()), ShouldEqual, first)
				So((*data.Measurement[float64])(tee.Next()), ShouldEqual, second)
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

		if (*data.Measurement[float64])(tee.Next()) != measurement {
			b.Fatal("measurement pointer changed")
		}
	}

	b.StopTimer()

	if err := tee.Close(); err != nil {
		b.Fatal(err)
	}
}
