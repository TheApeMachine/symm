package tables

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

/*
Replay is only worth anything if a replayed frame cannot be told from a live
one. A reader that resolves ticker.data.last against a live frame has to
resolve it against an archived one, which means what comes back out is the
payload itself and not the row that carried it.
*/
func TestFrameBytes(t *testing.T) {
	Convey("Given an archived frame", t, func() {
		Convey("It comes back as the bytes that went in", func() {
			frame, carried := frameBytes([]byte(`{"channel":"ticker"}`))

			So(carried, ShouldBeTrue)
			So(string(frame), ShouldEqual, `{"channel":"ticker"}`)
		})

		Convey("It is decoded when Arrow hands it over as base64", func() {
			frame, carried := frameBytes("eyJjaGFubmVsIjoidGlja2VyIn0=")

			So(carried, ShouldBeTrue)
			So(string(frame), ShouldEqual, `{"channel":"ticker"}`)
		})

		// A row with no frame is not a frame with no content: replaying it
		// as one would put a reading into the tape that never happened.
		Convey("A row carrying no frame is not a frame", func() {
			_, carried := frameBytes(nil)
			So(carried, ShouldBeFalse)

			_, carried = frameBytes([]byte{})
			So(carried, ShouldBeFalse)

			_, carried = frameBytes("")
			So(carried, ShouldBeFalse)
		})
	})
}

/*
The payload column is where a frame is kept. A table declared without one
cannot be replayed, and saying so is better than replaying nothing.
*/
func TestReplayCursor(t *testing.T) {
	Convey("Given frames read back out of a table", t, func() {
		server := NewIcebergScan()
		server.payloads = [][]byte{
			[]byte(`{"seq":1}`), []byte(`{"seq":2}`), []byte(`{"seq":3}`),
		}

		Convey("They are handed back one at a time, in the order captured", func() {
			So(server.cursor, ShouldEqual, 0)

			for expected := range server.payloads {
				So(string(server.payloads[server.cursor]), ShouldEqual,
					string(server.payloads[expected]))
				server.cursor++
			}

			So(server.cursor, ShouldEqual, len(server.payloads))
		})
	})
}
