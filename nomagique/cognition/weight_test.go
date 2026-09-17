package cognition_test

import (
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/cognition"
)

func TestPackedWeightRecord(t *testing.T) {
	Convey("The only cognitive weight fits exactly in its primitive 24-byte record", t, func() {
		weight := cognition.PackedWeight{Count: 37, Mass: 0.625, WriteStep: 91}
		pack := cognition.NewPack()
		inPack := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&weight))
		}

		var encoded [cognition.WeightSize]byte
		for out := range pack.Next(inPack) {
			encoded = *(*[cognition.WeightSize]byte)(out)
		}

		unpack := cognition.NewWeight()
		inUnpack := func(yield func(unsafe.Pointer) bool) {
			rec := encoded[:]
			yield(unsafe.Pointer(&rec))
		}

		var decoded cognition.PackedWeight
		for out := range unpack.Next(inUnpack) {
			decoded = *(*cognition.PackedWeight)(out)
		}

		So(int(unsafe.Sizeof(weight)), ShouldEqual, cognition.WeightSize)
		So(decoded, ShouldResemble, weight)
	})
}

func TestWeightUnpacks(t *testing.T) {
	Convey("The Weight primitive unpacks stored records", t, func() {
		weight := cognition.PackedWeight{Count: 5, Mass: 0.5, WriteStep: 12}
		pack := cognition.NewPack()
		inPack := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&weight))
		}

		var encoded [cognition.WeightSize]byte
		for out := range pack.Next(inPack) {
			encoded = *(*[cognition.WeightSize]byte)(out)
		}

		unpack := cognition.NewWeight()
		inUnpack := func(yield func(unsafe.Pointer) bool) {
			rec := encoded[:]
			yield(unsafe.Pointer(&rec))
		}

		var decoded cognition.PackedWeight
		for out := range unpack.Next(inUnpack) {
			decoded = *(*cognition.PackedWeight)(out)
		}

		So(unpack.Error(), ShouldBeNil)
		So(decoded, ShouldResemble, weight)
	})

	Convey("A short record is a shape failure rather than a zero weight", t, func() {
		unpack := cognition.NewWeight()
		short := make([]byte, 8)
		in := func(yield func(unsafe.Pointer) bool) {
			yield(unsafe.Pointer(&short))
		}

		for range unpack.Next(in) {
		}

		So(unpack.Error(), ShouldNotBeNil)
	})
}

func TestKeyLayout(t *testing.T) {
	Convey("The Key primitive builds and parses the store's key layout", t, func() {
		keys := cognition.NewKey()
		var results []cognition.KeyResult

		commands := []cognition.KeyCommand{
			{Basin: &cognition.Basin{Class: []byte("enter"), Context: []byte("ctx")}},
			{Sensory: &cognition.Sensory{Context: []byte("ctx")}},
			{Parse: []byte("b/ctx/enter")},
			{Parse: []byte("s/ctx")},
		}

		for index := range commands {
			cmd := commands[index]
			in := func(yield func(unsafe.Pointer) bool) {
				yield(unsafe.Pointer(&cmd))
			}
			for out := range keys.Next(in) {
				results = append(results, *(*cognition.KeyResult)(out))
			}
		}

		So(keys.Error(), ShouldBeNil)
		So(len(results), ShouldEqual, 4)
		So(string(results[0].Key), ShouldEqual, "b/ctx/enter")
		So(string(results[1].Key), ShouldEqual, "s/ctx")
		So(string(results[2].Class), ShouldEqual, "enter")
		So(string(results[2].Context), ShouldEqual, "ctx")
		So(results[2].Valid, ShouldBeTrue)
	})
}
