package core_test

import (
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data/sequence"
)

func TestInputNext(t *testing.T) {
	Convey("Input borrows payloads without accumulating or copying them", t, func() {
		value := 4.0
		input := core.NewInput[string](nil, core.NewAction(core.ActionWrite), "price", &value)

		Convey("A configured request preserves its key and exact payload address", func() {
			count := 0
			for output := range input.Next(nil) {
				request := (*core.Input[string, string, float64])(output)
				So(request, ShouldEqual, input)
				So(request.Key, ShouldEqual, "price")
				So(request.Value, ShouldEqual, &value)
				count++
			}
			So(count, ShouldEqual, 1)
		})

		Convey("Incoming payloads bind one at a time and stop with the consumer", func() {
			values := []float64{1, 5, -2}
			count := 0
			for output := range input.Next(sequence.NewValue(values...)) {
				request := (*core.Input[string, string, float64])(output)
				So(request.Value, ShouldEqual, &values[count])
				count++
				if count == 2 {
					break
				}
			}
			So(count, ShouldEqual, 2)
			So(input.Value, ShouldEqual, &values[1])
		})

		Convey("An empty upstream produces no extra request", func() {
			count := 0
			for range input.Next(sequence.NewValue[float64]()) {
				count++
			}
			So(count, ShouldEqual, 0)
		})

		Convey("A read has an explicit absence of a write payload", func() {
			read := core.NewInput[string, string, float64](nil, core.NewAction(core.ActionRead), "price", nil)
			for output := range read.Next(nil) {
				So((*core.Input[string, string, float64])(output).Value, ShouldBeNil)
			}
		})
	})
}

func BenchmarkInputNext(b *testing.B) {
	value := 1.0
	input := core.NewInput[string](nil, core.NewAction(core.ActionWrite), "price", &value)
	b.ReportAllocs()
	for b.Loop() {
		for output := range input.Next(nil) {
			if output != unsafe.Pointer(input) {
				b.Fatal("request was copied")
			}
		}
	}
}
