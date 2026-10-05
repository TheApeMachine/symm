package data

import (
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/nomagique/core"
)

func TestAdapterNext(t *testing.T) {
	Convey("Given an adapter", t, func() {
		state := NewState(NewMap("value", "value", "context", "context"))
		adapter := NewAdapter(nil, state)

		Convey("It publishes and reads numbers", func() {
			issued := NewOutputMap()
			issued.Values["value"] = 3

			for range adapter.Next(NewValue(issued)) {
			}

			So(adapter.Error(), ShouldBeNil)

			var got Map[float64]

			for pointer := range adapter.Next(NewValue(NewMap("value", "value"))) {
				got = *(*Map[float64])(pointer)
			}

			So(adapter.Error(), ShouldBeNil)
			So(got.Values["value"], ShouldEqual, 3)
		})

		Convey("It publishes and reads text", func() {
			issued := NewTextMap()
			issued.Values["context"] = "aa/bb"

			for range adapter.Next(NewValue(issued)) {
			}

			So(adapter.Error(), ShouldBeNil)

			var got Map[string]

			for pointer := range adapter.Next(NewValue(NewLiteral("context"))) {
				got = *(*Map[string])(pointer)
			}

			So(adapter.Error(), ShouldBeNil)
			So(got.Values["context"], ShouldEqual, "aa/bb")
		})

		Convey("Missing text stays missing", func() {
			for range adapter.Next(NewValue(NewLiteral("absent"))) {
			}

			So(errors.Is(adapter.Error(), core.ErrNotHeld), ShouldBeTrue)
		})
	})
}
