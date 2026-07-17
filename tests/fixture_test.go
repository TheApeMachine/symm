package tests

import (
	"iter"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFrameSequence(t *testing.T) {
	Convey("Given a Kraken-shaped payload sequence", t, func() {
		sequence := func(yield func([]byte) bool) {
			yield([]byte(`{"channel":"ticker","type":"snapshot","data":[{"symbol":"MATIC/USD"}]}`))
		}

		Convey("When frames are requested", func() {
			frames := FrameSequence(iter.Seq[[]byte](sequence))
			count := 0

			for frame := range frames {
				count++

				So(frame.Channel, ShouldEqual, "ticker")
				So(frame.Type, ShouldEqual, "snapshot")
				So(frame.Payload, ShouldNotBeEmpty)
			}

			Convey("Then every frame should use channel and type directly", func() {
				So(count, ShouldEqual, 1)
			})
		})
	})
}
