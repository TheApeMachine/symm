package manifold

import (
	"encoding/binary"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestEncodeDisplay(t *testing.T) {
	Convey("Given a valid GPU RGBA8 display", t, func() {
		rgba := []byte{
			10, 20, 30, 255,
			40, 50, 60, 255,
			70, 80, 90, 255,
			100, 110, 120, 255,
		}
		at := time.Unix(1, 2).UTC()

		payload, err := EncodeDisplay("BTC/USD", at, 2, 2, rgba)

		Convey("It should encode a complete SMF1 display frame", func() {
			So(err, ShouldBeNil)
			So(string(payload[:4]), ShouldEqual, "SMF1")
			So(payload[4], ShouldEqual, binaryKindDisplay)
			So(binary.LittleEndian.Uint16(payload[5:7]), ShouldEqual, 2)
			So(binary.LittleEndian.Uint16(payload[7:9]), ShouldEqual, 2)
			So(int64(binary.LittleEndian.Uint64(payload[17:25])), ShouldEqual, at.UnixNano())
			So(string(payload[26:33]), ShouldEqual, "BTC/USD")
			So(payload[33:], ShouldResemble, rgba)
		})
	})

	Convey("Given RGBA8 bytes that do not match the dimensions", t, func() {
		_, err := EncodeDisplay("BTC/USD", time.Now().UTC(), 2, 2, []byte{1})

		Convey("It should reject the malformed display", func() {
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "1 bytes for 2x2 RGBA8")
		})
	})
}
