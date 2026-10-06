package arithmetic

import (
	"errors"
	"testing"
	"unsafe"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestBinaryNext(t *testing.T) {
	Convey("Given the adapter-native binary field operations", t, func() {
		factories := map[string]func() core.Primitive{
			"add": NewAdd, "subtract": NewSubtract, "multiply": NewMultiply, "divide": NewDivide,
		}
		references := map[string]func(float64, float64) float64{
			"add":      func(l, r float64) float64 { return l + r },
			"subtract": func(l, r float64) float64 { return l - r },
			"multiply": func(l, r float64) float64 { return l * r },
			"divide":   func(l, r float64) float64 { return l / r },
		}

		for name, factory := range factories {
			reference := references[name]

			Convey("It publishes "+name+" as value and "+name, func() {
				op := factory()

				for _, pair := range [][2]float64{{3, 4}, {-2.5, 0.5}, {1e9, 1e-3}} {
					shared := data.NewOutputMap()
					shared.Values["a"] = pair[0]
					shared.Values["b"] = pair[1]
					adapter := data.NewAdapter(nil, data.NewState(
						data.NewMap("left", "a", "right", "b", name, "out"), shared,
					))

					for range op.Next(data.NewValue(adapter)) {
					}

					So(op.Error(), ShouldBeNil)
					So(shared.Values["out"], ShouldEqual, reference(pair[0], pair[1]))
					So(shared.Values["value"], ShouldEqual, reference(pair[0], pair[1]))
				}
			})

			Convey(name+" records ErrNotHeld for a missing operand", func() {
				op := factory()
				shared := data.NewOutputMap()
				shared.Values["left"] = 1
				adapter := data.NewAdapter(nil, data.NewState(data.NewMap(), shared))

				for range op.Next(data.NewValue(adapter)) {
				}

				So(errors.Is(op.Error(), core.ErrNotHeld), ShouldBeTrue)
			})

			Convey(name+" records ErrShape for a nil arrival", func() {
				op := factory()

				for range op.Next(func(yield func(unsafe.Pointer) bool) { yield(nil) }) {
				}

				So(errors.Is(op.Error(), core.ErrShape), ShouldBeTrue)
			})
		}

		Convey("Divide leaves a zero-divisor quotient unwritten and hands the arrival on", func() {
			op := NewDivide()
			shared := data.NewOutputMap()
			shared.Values["left"] = 1
			shared.Values["right"] = 0
			adapter := data.NewAdapter(nil, data.NewState(data.NewMap(), shared))
			handed := 0

			for range op.Next(data.NewValue(adapter)) {
				handed++
			}

			So(op.Error(), ShouldBeNil)
			So(handed, ShouldEqual, 1)
			_, written := shared.Values["divide"]
			So(written, ShouldBeFalse)
		})
	})
}
