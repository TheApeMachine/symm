package cognition

import (
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/nomagique/transport"
)

func TestPackedWeightRecord(t *testing.T) {
	Convey("The only cognitive weight fits exactly in its primitive 24-byte record", t, func() {
		weight := PackedWeight{Count: 37, Probability: 0.625, WriteStep: 91}
		var encoded [WeightSize]byte
		encodeWeight(encoded[:], weight)
		So(int(unsafe.Sizeof(weight)), ShouldEqual, WeightSize)
		So(decodeWeight(encoded[:]), ShouldResemble, weight)
	})
}

func TestWeightUnpacks(t *testing.T) {
	Convey("The Weight primitive unpacks stored records", t, func() {
		weight := PackedWeight{Count: 5, Probability: 0.5, WriteStep: 12}
		var encoded [WeightSize]byte
		encodeWeight(encoded[:], weight)

		unpack := NewWeight()
		var unpacked PackedWeight

		for out := range unpack.Next(transport.NewValues(WeightRecord(encoded[:])).Next(nil)) {
			unpacked = *(*PackedWeight)(out)
		}

		So(unpack.Error(), ShouldBeNil)
		So(unpacked, ShouldResemble, weight)
	})

	Convey("A short record is a shape failure rather than a zero weight", t, func() {
		unpack := NewWeight()
		short := WeightRecord(make([]byte, 8))

		for range unpack.Next(transport.NewValues(short).Next(nil)) {
		}

		So(unpack.Error(), ShouldNotBeNil)
	})
}

func TestPackedWeightReinforce(t *testing.T) {
	Convey("Signed feedback updates the same association with its actual magnitude", t, func() {
		graded := func(feedback float64) PackedWeight {
			weight := PackedWeight{Count: 1, Probability: 0.5}
			reinforce(&weight, feedback, true)
			return weight
		}

		So(graded(1).Probability, ShouldEqual, 0.75)
		So(graded(-1).Probability, ShouldEqual, 0.25)
		So(graded(-3).Probability, ShouldEqual, 0.125)
		So(graded(0).Probability, ShouldEqual, 0.5)

		Convey("The original association-only call strengthens by one unit", func() {
			weight := PackedWeight{Count: 1, Probability: 0.5}
			reinforce(&weight, 0, false)
			So(weight.Probability, ShouldEqual, 0.75)
		})
	})
}

func TestPackedWeightEffective(t *testing.T) {
	Convey("Decay preserves a seen record while reducing its strength", t, func() {
		weight := PackedWeight{Count: 1, Probability: 0.5, WriteStep: 10}
		So(weight.effective(12, 0.5).Count, ShouldEqual, 1)
		So(weight.effective(12, 0.5).Probability, ShouldEqual, 0.125)
		So(weight.effective(10, 0.5), ShouldResemble, weight)
		So(weight.effective(12, 1), ShouldResemble, weight)
	})
}

func TestKeyLayout(t *testing.T) {
	Convey("The Key primitive builds and parses the store's key layout", t, func() {
		keys := NewKey()
		var results []KeyResult

		commands := []KeyCommand{
			{Basin: &Basin{Class: []byte("enter"), Context: []byte("ctx")}},
			{Sensory: &Sensory{Context: []byte("ctx")}},
			{Parse: makeBasinKey([]byte("enter"), []byte("ctx"))},
			{Parse: []byte("s/ctx")},
			{Parse: []byte("o/ctx/enter")},
		}

		for index := range commands {
			for out := range keys.Next(transport.NewOne(unsafe.Pointer(&commands[index])).Next(nil)) {
				results = append(results, *(*KeyResult)(out))
			}
		}

		So(keys.Error(), ShouldBeNil)
		So(len(results), ShouldEqual, 5)
		So(string(results[0].Key), ShouldEqual, "b/ctx/enter")
		So(string(results[1].Key), ShouldEqual, "s/ctx")
		So(string(results[2].Class), ShouldEqual, "enter")
		So(string(results[2].Context), ShouldEqual, "ctx")
		So(results[2].Valid, ShouldBeTrue)
		So(results[3].Valid, ShouldBeFalse)
		So(results[4].Valid, ShouldBeFalse)
	})

	Convey("A key command must set exactly one intent", t, func() {
		keys := NewKey()
		command := KeyCommand{}

		for range keys.Next(transport.NewOne(unsafe.Pointer(&command)).Next(nil)) {
		}

		So(keys.Error(), ShouldNotBeNil)
	})
}

func BenchmarkPackedWeightReinforce(b *testing.B) {
	weight := PackedWeight{Count: 1, Probability: 0.5}
	b.ReportAllocs()

	for b.Loop() {
		reinforce(&weight, -0.01, true)
		reinforce(&weight, 0.02, true)
	}
}
