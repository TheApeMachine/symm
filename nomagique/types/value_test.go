package types

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewValue(t *testing.T) {
	Convey("Given a written payload", t, func() {
		payload := NewValue(3.5)
		input := NewInput[float64]()
		input.(*InputValue[float64]).Value = payload

		Convey("It should read correctly", func() {
			So(payload.Error(), ShouldBeBlank)
			So(payload.Read(), ShouldEqual, 3.5)
			So(input.Project().Read(), ShouldEqual, 3.5)
		})
	})
}

func TestNewInput(t *testing.T) {
	Convey("Given an empty holder", t, func() {
		holder := NewInput[float64]()

		Convey("It should read zero until staged", func() {
			So(holder.Project().Read(), ShouldEqual, 0)
			holder.(*InputValue[float64]).Value = NewValue(2.0)
			So(holder.Project().Read(), ShouldEqual, 2.0)
		})
	})
}

func TestWrite(t *testing.T) {
	Convey("Given two numeric values", t, func() {
		source := NewInput[float64]()
		source.(*InputValue[float64]).Value = NewValue(4.0)
		dest := NewInput[float64]()

		Convey("It should stage the source number", func() {
			dest.Write(source)
			So(dest.Project().Read(), ShouldEqual, 4.0)
		})
	})
}

func TestReset(t *testing.T) {
	Convey("Given a ready value", t, func() {
		payload := NewValue(1.0)

		Convey("It should clear the staged payload", func() {
			So(payload.Reset(), ShouldBeNil)
			So(payload.Read(), ShouldEqual, 1.0)
		})
	})
}

func TestClose(t *testing.T) {
	Convey("Given a ready value", t, func() {
		payload := NewValue(1.0)

		Convey("It should release staged state", func() {
			So(payload.Close(), ShouldBeNil)
			So(payload.Error(), ShouldBeBlank)
		})
	})
}
