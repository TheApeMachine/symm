package manifold

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestEncodeDisplay(t *testing.T) {
	Convey("Given a baked RGBA tile", t, func() {
		rgba := []byte{
			10, 20, 30, 255,
			40, 50, 60, 255,
			70, 80, 90, 255,
			100, 110, 120, 255,
		}

		Convey("It encodes one SMF1 display frame", func() {
			payload, ok := EncodeDisplay(
				"BTC/USD", time.Unix(1, 0).UTC(), 2, 2, rgba,
			)
			So(ok, ShouldBeTrue)
			key, known := BinaryCacheKey(payload)
			So(known, ShouldBeTrue)
			So(key, ShouldEqual, "manifold_display")
			So(payload[4], ShouldEqual, BinaryKindDisplay)
			So(payload[len(payload)-4:], ShouldResemble, []byte{100, 110, 120, 255})
		})
	})
}
