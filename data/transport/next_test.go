package transport

import (
	"bytes"
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/data"
)

func TestNextYieldsEachValue(t *testing.T) {
	Convey("Given two values written one after another", t, func() {
		first := data.NewValue("kraken", "trade", "BTC")
		second := data.NewValue("kraken", "trade", "ETH")
		reader := bytes.NewReader(append(first.Bytes(), second.Bytes()...))

		Convey("When ranging over Next", func() {
			var scopes []string

			for value := range Next(reader) {
				So(value.Err, ShouldBeNil)
				scopes = append(scopes, value.Scope)
			}

			Convey("Then each value is yielded in order", func() {
				So(scopes, ShouldResemble, []string{"BTC", "ETH"})
			})
		})
	})
}

func TestNextStopsWhenTheCallerStops(t *testing.T) {
	Convey("Given two values written one after another", t, func() {
		first := data.NewValue("kraken", "trade", "BTC")
		second := data.NewValue("kraken", "trade", "ETH")
		reader := bytes.NewReader(append(first.Bytes(), second.Bytes()...))

		Convey("When the caller breaks after the first value", func() {
			var scopes []string

			for value := range Next(reader) {
				scopes = append(scopes, value.Scope)
				break
			}

			Convey("Then the second value is left unread", func() {
				So(scopes, ShouldResemble, []string{"BTC"})
				So(reader.Len(), ShouldEqual, len(second.Bytes()))
			})
		})
	})
}

func TestNextEmptyReader(t *testing.T) {
	Convey("Given an empty reader", t, func() {
		var count int

		for range Next(bytes.NewReader(nil)) {
			count++
		}

		Convey("Then Next yields nothing", func() {
			So(count, ShouldEqual, 0)
		})
	})
}
