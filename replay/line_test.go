package replay

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestLineConstants(t *testing.T) {
	Convey("Given replay transport constants", t, func() {
		Convey("It should distinguish ws and rest transports", func() {
			So(TransportWS, ShouldEqual, "ws")
			So(TransportREST, ShouldEqual, "rest")
			So(DirectionIn, ShouldEqual, "in")
		})
	})
}

func TestLineStruct(t *testing.T) {
	Convey("Given a JSONL line", t, func() {
		line := Line{
			Transport: TransportWS,
			Channel:   "ticker",
			Direction: DirectionIn,
			Payload:   []byte(`{"channel":"ticker"}`),
		}

		Convey("It should retain transport metadata", func() {
			So(line.Transport, ShouldEqual, TransportWS)
			So(line.Channel, ShouldEqual, "ticker")
		})
	})
}
