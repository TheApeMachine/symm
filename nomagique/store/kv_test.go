package store_test

import (
	"errors"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/tests"
	"golang.design/x/lockfree/lf"
)

func TestKVNext(t *testing.T) {
	Convey("KV reads and writes the supplied store through keyed Input requests", t, func() {
		backing := lf.NewOrderedMap[string, float64](func(left, right string) bool { return left < right })
		kv := store.NewKV[string](backing)
		payload := 7.0
		write := core.NewInput[string](nil, core.NewAction(core.ActionWrite), "price", &payload)
		read := core.NewInput[string, string, float64](nil, core.NewAction(core.ActionRead), "price", nil)

		Convey("Writes are visible both through reads and through the original store", func() {
			for _, value := range []float64{7, 12, -3, 0} {
				payload = value
				for output := range kv.Next(write.Next(nil)) {
					So(output, ShouldEqual, unsafe.Pointer(&payload))
				}
				So(tests.CollectSeq[float64](kv.Next(read.Next(nil))), ShouldResemble, []float64{value})
				stored, found := backing.Get("price")
				So(found, ShouldBeTrue)
				So(stored, ShouldEqual, value)
			}
			So(kv.Error(), ShouldBeNil)
		})

		Convey("The zero key and a zero value are ordinary data", func() {
			write.Key, payload = "", 0
			for range kv.Next(write.Next(nil)) {
			}
			read.Key = ""
			So(tests.CollectSeq[float64](kv.Next(read.Next(nil))), ShouldResemble, []float64{0})
			So(kv.Error(), ShouldBeNil)
		})

		Convey("Write then read executes the declared action order", func() {
			write.Action = core.NewAction(core.ActionWrite, core.ActionRead)
			So(tests.CollectSeq[float64](kv.Next(write.Next(nil))), ShouldResemble, []float64{7, 7})
		})

		Convey("Stopping after the first action does not execute the next one", func() {
			backing.Set("price", 3)
			write.Action = core.NewAction(core.ActionRead, core.ActionWrite)
			for output := range kv.Next(write.Next(nil)) {
				So(*(*float64)(output), ShouldEqual, 3)
				break
			}
			stored, _ := backing.Get("price")
			So(stored, ShouldEqual, 3)
		})

		Convey("Missing keys produce an error and no invented zero", func() {
			So(tests.CollectSeq[float64](kv.Next(read.Next(nil))), ShouldBeEmpty)
			So(errors.Is(kv.Error(), core.ErrNotHeld), ShouldBeTrue)
		})

		Convey("Writes without a payload fail without altering the map", func() {
			write.Value = nil
			So(tests.CollectSeq[float64](kv.Next(write.Next(nil))), ShouldBeEmpty)
			So(errors.Is(kv.Error(), core.ErrShape), ShouldBeTrue)
			_, found := backing.Get("price")
			So(found, ShouldBeFalse)
		})

		Convey("Missing, empty, and unsupported action requests are explicit failures", func() {
			for _, action := range []*core.Action{nil, core.NewAction(), core.NewAction(core.ActionExecute)} {
				current := store.NewKV[string](backing)
				write.Action = action
				So(tests.CollectSeq[float64](current.Next(write.Next(nil))), ShouldBeEmpty)
				So(errors.Is(current.Error(), core.ErrDomain), ShouldBeTrue)
			}
		})

		Convey("A composed input stage binds every arriving value", func() {
			pipeline := nomagique.NewNumber(write, kv)
			So(tests.CollectSeq[float64](pipeline.Next(sequence.NewValue(1.0, 5.0, -2.0))), ShouldResemble, []float64{1, 5, -2})
			So(tests.CollectSeq[float64](kv.Next(read.Next(nil))), ShouldResemble, []float64{-2})
			So(pipeline.Error(), ShouldBeNil)
		})
	})
}

func BenchmarkKVNext(b *testing.B) {
	backing := lf.NewOrderedMap[string, float64](func(left, right string) bool { return left < right })
	backing.Set("price", 0)
	kv := store.NewKV[string](backing)
	value := 0.0
	request := core.NewInput[string](nil, core.NewAction(core.ActionWrite, core.ActionRead), "price", &value)
	input := request.Next(nil)
	b.ReportAllocs()
	for b.Loop() {
		value++
		count := 0
		for output := range kv.Next(input) {
			if *(*float64)(output) != value {
				b.Fatal("read differs from write")
			}
			count++
		}
		if count != 2 {
			b.Fatal(count)
		}
	}
}
