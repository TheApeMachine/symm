package market

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestTrainingTape(t *testing.T) {
	Convey("Given a requested training tape", t, func() {
		frames := TrainingTape(2)

		Convey("it expands into stable volume regime frames with all expected peers", func() {
			So(len(frames), ShouldBeGreaterThan, 0)

			for _, frame := range frames {
				So(frame.Label, ShouldEqual, "BTC/USD")
				So(frame.Source, ShouldEqual, "training")

				prevInput := data.Pull(frame.Read("previous_input"))
				So(prevInput.Metric.Label, ShouldEqual, "previous_input")

				peers := frame.Peers()
				So(len(peers), ShouldEqual, 3)

				trade := peers[0]
				So(trade.Source, ShouldEqual, "spot:trade")
				So(trade.Meta("type"), ShouldEqual, "trade")
				So(trade.Meta("side"), ShouldBeIn, "buy", "sell")
				So(data.Pull(trade.Read("price")).Metric.Exact, ShouldNotBeNil)
				So(data.Pull(trade.Read("qty")).Metric.Exact, ShouldNotBeNil)

				// Touch never rides a pipeline frame.
				for _, peer := range peers {
					for _, key := range []string{"bid", "ask"} {
						So(data.Pull(peer.Read(key)), ShouldBeNil)
					}
				}

				So(peers[1].Source, ShouldEqual, "direct")
				So(peers[2].Source, ShouldEqual, "inverse")
			}
		})
	})
}
