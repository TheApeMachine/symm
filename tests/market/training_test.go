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
				So(len(peers), ShouldEqual, 4)

				quote := peers[0]
				So(quote.Source, ShouldEqual, "quote")
				So(data.Pull(quote.Read("bid")).Metric.Label, ShouldEqual, "bid")
				So(data.Pull(quote.Read("ask")).Metric.Label, ShouldEqual, "ask")

				trade := peers[1]
				So(trade.Source, ShouldEqual, "public")
				So(data.Pull(trade.Read("price")).Metric.Label, ShouldEqual, "price")
			}
		})
	})
}
