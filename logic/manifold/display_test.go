package manifold

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	pfluid "github.com/theapemachine/nomagique/physics/fluid"
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

func TestStateWirePackets(t *testing.T) {
	Convey("Given a manifold state with a baked GPU display", t, func() {
		state := State{
			Source:        "manifold",
			Symbol:        "BTC/USD",
			At:            time.Unix(2, 0).UTC(),
			DisplayWidth:  2,
			DisplayHeight: 2,
			Display: []byte{
				10, 20, 30, 255,
				40, 50, 60, 255,
				70, 80, 90, 255,
				100, 110, 120, 255,
			},
			Wave: []pfluid.WaveMode{{Omega: 1.5}},
			PhaseReady: true,
		}

		Convey("It splits summary, binary display, and wave packet", func() {
			field, displays, wave := state.WirePackets()
			So(field.Display, ShouldBeNil)
			So(field.DisplayWidth, ShouldEqual, 0)
			So(len(displays), ShouldEqual, 1)
			So(wave.Symbol, ShouldEqual, "BTC/USD")
			So(wave.Ready, ShouldBeTrue)
			So(len(wave.Wave), ShouldEqual, 1)

			key, known := BinaryCacheKey(displays[0])
			So(known, ShouldBeTrue)
			So(key, ShouldEqual, "manifold_display")
		})
	})
}
