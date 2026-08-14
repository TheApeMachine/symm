package types

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewPair(t *testing.T) {
	Convey("Given a key and a value", t, func() {
		pair := NewPair("event_count", 7.0)

		Convey("It should hold that one position", func() {
			So(pair.Key, ShouldEqual, "event_count")
			So(pair.Value, ShouldEqual, 7)
		})
	})
}
