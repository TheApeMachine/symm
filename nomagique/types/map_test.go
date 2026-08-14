package types

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewMap(t *testing.T) {
	Convey("Given a new map", t, func() {
		collected := NewMap[string, float64]()

		Convey("It should be empty and comparable as IO's T", func() {
			So(collected.Equals(NewMap[string, float64]()), ShouldBeTrue)
			var input Input[Map[string, float64]] = NewInput[Map[string, float64]]()
			So(input, ShouldNotBeNil)
		})
	})
}

func TestGet(t *testing.T) {
	Convey("Given a map with one pair", t, func() {
		collected := NewMap[string, float64]()
		collected.Put("event_count", 7)

		Convey("Get should return the value", func() {
			value, found := collected.Get("event_count")
			So(found, ShouldBeTrue)
			So(value, ShouldEqual, 7)
		})
	})
}

func TestPut(t *testing.T) {
	Convey("Given an empty map", t, func() {
		collected := NewMap[string, float64]()

		Convey("Put should store the pair", func() {
			collected.Put("maturity", 0.4)
			value, found := collected.Get("maturity")
			So(found, ShouldBeTrue)
			So(value, ShouldEqual, 0.4)
		})
	})
}

func TestEquals(t *testing.T) {
	Convey("Given two maps with the same pairs", t, func() {
		left := NewMap[string, float64]()
		right := NewMap[string, float64]()
		left.Put("event_count", 7)
		right.Put("event_count", 7)

		Convey("Equals should report them as the same collection", func() {
			So(left.Equals(right), ShouldBeTrue)
		})
	})

	Convey("Given two maps that differ in a value", t, func() {
		left := NewMap[string, float64]()
		right := NewMap[string, float64]()
		left.Put("event_count", 7)
		right.Put("event_count", 8)

		Convey("Equals should report them as different", func() {
			So(left.Equals(right), ShouldBeFalse)
		})
	})
}
