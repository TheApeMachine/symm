package ring

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFloatRingPushOverwrite(t *testing.T) {
	Convey("Given a full ring buffer", t, func() {
		ringBuffer := NewFloatRing(3)

		ringBuffer.Push(1)
		ringBuffer.Push(2)
		ringBuffer.Push(3)
		ringBuffer.Push(4)

		Convey("It should retain the newest values in order", func() {
			So(ringBuffer.Len(), ShouldEqual, 3)
			So(ringBuffer.Ordered(), ShouldResemble, []float64{2, 3, 4})
		})
	})
}

func BenchmarkFloatRingPush(b *testing.B) {
	ringBuffer := NewFloatRing(24)

	b.ReportAllocs()

	for b.Loop() {
		ringBuffer.Push(1.23)
	}
}
