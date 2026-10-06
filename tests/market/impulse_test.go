package market

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestImpulseTape(t *testing.T) {
	Convey("Given a requested impulse tape", t, func() {
		frames := ImpulseTape("BTC/USD", 2)

		Convey("it generates the expected frames and peers", func() {
			So(len(frames), ShouldBeGreaterThan, 0)

			for _, frame := range frames {
				So(frame.Label, ShouldEqual, "BTC/USD")
				So(frame.Source, ShouldEqual, "frame")

				peers := frame.Peers()
				So(len(peers), ShouldEqual, 3)

				names := make([]string, 0, len(peers))

				for _, peer := range peers {
					So(peer.Label, ShouldEqual, "BTC/USD")
					names = append(names, peer.Source)

					valEntry := data.Pull(peer.Read("value"))
					So(valEntry.Metric.Label, ShouldEqual, "value")
					So(valEntry.Metric.Raw, ShouldNotEqual, 0)
				}

				So(names, ShouldResemble, []string{"public", "direct", "inverse"})
			}
		})
	})
}
