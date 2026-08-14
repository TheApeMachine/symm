package types

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewValue(t *testing.T) {
	Convey("Given a written payload", t, func() {
		payload := NewValue(3.5)
		var input Input[float64] = payload
		var output Output[float64] = payload

		Convey("It should be a legal input and a legal output", func() {
			So(input.Error(), ShouldBeNil)
			So(output.Error(), ShouldBeNil)
			So(input.Value(), ShouldEqual, 3.5)
			So(output.Value(), ShouldEqual, 3.5)
		})
	})
}

func TestNewInput(t *testing.T) {
	Convey("Given an empty holder", t, func() {
		holder := NewInput[float64]()

		Convey("It should error until staged", func() {
			So(holder.Value(), ShouldEqual, 0)
			So(holder.Error(), ShouldNotBeNil)
			holder.Store(2)
			So(holder.Value(), ShouldEqual, 2)
			So(holder.Error(), ShouldBeNil)
		})
	})
}

func TestNewFailed(t *testing.T) {
	Convey("Given a failed holder", t, func() {
		failed := NewFailed[float64]("value: missing")

		Convey("It should report the error without inventing a payload", func() {
			So(failed.Value(), ShouldEqual, 0)
			So(failed.Error(), ShouldNotBeNil)
		})
	})
}

func TestWrite(t *testing.T) {
	Convey("Given two numeric values", t, func() {
		source := NewValue(4.0)
		dest := NewInput[float64]()

		Convey("It should stage the source number", func() {
			So(dest.Write(source), ShouldBeNil)
			So(dest.Value(), ShouldEqual, 4)
		})
	})
}

func TestRead(t *testing.T) {
	Convey("Given a ready value", t, func() {
		source := NewValue(7.0)
		dest := NewInput[float64]()

		Convey("It should copy the payload into dest", func() {
			So(source.Read(dest), ShouldBeNil)
			So(dest.Value(), ShouldEqual, 7)
		})
	})
}

func TestReset(t *testing.T) {
	Convey("Given a ready value", t, func() {
		payload := NewValue(1.0)

		Convey("It should clear the staged payload", func() {
			So(payload.Reset(), ShouldBeNil)
			So(payload.Value(), ShouldEqual, 0)
			So(payload.Error(), ShouldNotBeNil)
		})
	})
}

func TestClose(t *testing.T) {
	Convey("Given a ready value", t, func() {
		payload := NewValue(1.0)

		Convey("It should release staged state", func() {
			So(payload.Close(), ShouldBeNil)
			So(payload.Error(), ShouldBeNil)
		})
	})
}
