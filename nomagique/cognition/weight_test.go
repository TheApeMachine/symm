package cognition

import (
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPackedWeightEncode(t *testing.T) {
	Convey("The only cognitive weight fits exactly in its primitive 24-byte record", t, func() {
		weight := PackedWeight{Count: 37, Probability: 0.625, WriteStep: 91}
		var encoded [WeightSize]byte
		weight.Encode(encoded[:])
		So(unsafe.Sizeof(weight), ShouldEqual, WeightSize)
		So(DecodeWeight(encoded[:]), ShouldResemble, weight)
	})
}

func TestPackedWeightReinforce(t *testing.T) {
	Convey("Signed feedback updates the same association with its actual magnitude", t, func() {
		weight := PackedWeight{Count: 1, Probability: 0.5}
		Convey("Profitable feedback increases attraction", func() {
			weight.Reinforce(1)
			So(weight.Probability, ShouldEqual, 0.75)
		})
		Convey("Loss feedback inhibits the action", func() {
			weight.Reinforce(-1)
			So(weight.Probability, ShouldEqual, 0.25)
		})
		Convey("Larger losses inhibit more", func() {
			weight.Reinforce(-3)
			So(weight.Probability, ShouldEqual, 0.125)
		})
		Convey("Zero does not reward inactivity", func() {
			weight.Reinforce(0)
			So(weight.Probability, ShouldEqual, 0.5)
		})
		Convey("The original association-only call remains supported", func() {
			weight.Reinforce()
			So(weight.Probability, ShouldEqual, 0.75)
		})
	})
}

func TestPackedWeightEffective(t *testing.T) {
	Convey("Decay preserves a seen record while reducing its strength", t, func() {
		weight := PackedWeight{Count: 1, Probability: 0.5, WriteStep: 10}
		So(weight.Effective(12, 0.5).Count, ShouldEqual, 1)
		So(weight.Effective(12, 0.5).Probability, ShouldEqual, 0.125)
		So(weight.Effective(10, 0.5), ShouldResemble, weight)
		So(weight.Effective(12, 1), ShouldResemble, weight)
	})
}

func BenchmarkPackedWeightReinforce(b *testing.B) {
	weight := PackedWeight{Count: 1, Probability: 0.5}
	b.ReportAllocs()
	for b.Loop() {
		weight.Reinforce(-0.01)
		weight.Reinforce(0.02)
	}
}
