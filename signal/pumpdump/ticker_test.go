package pumpdump

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/nomagique"
)

func TestTickerPipelineScoresProductionConfig(testingTB *testing.T) {
	Convey("Given the production pumpdump ticker pipeline", testingTB, func() {
		ticker := NewTicker()
		timestamp := time.Unix(0, 1).UnixNano()

		for index := range 24 {
			frame := tickerPipelineFrame(
				1000+float64(index)*10,
				10000+float64(index)*100,
				timestamp,
			)

			So(nomagique.RoundTripArtifact(frame, ticker.algo), ShouldBeNil)
			frame.Release()
			timestamp += int64(time.Second)
		}

		spike := tickerPipelineFrame(5000, 20000, timestamp)
		err := nomagique.RoundTripArtifact(spike, ticker.algo)

		Convey("It should publish classifier output instead of falling back to the neutral baseline", func() {
			So(err, ShouldBeNil)
			So(datura.Peek[float64](spike, "output", "ignition"), ShouldBeGreaterThan, 0)
			So(datura.Peek[float64](spike, "output", "confidence"), ShouldBeGreaterThan, 0.25)
			So(datura.Peek[[]float64](spike, "output", "probabilities"), ShouldNotResemble, []float64{
				0.25,
				0.25,
				0.25,
				0.25,
			})
		})

		spike.Release()
	})
}

func tickerPipelineFrame(volume, last float64, timestamp int64) *datura.Artifact {
	frame := datura.Acquire("pumpdump-ticker-pipeline", datura.APPJSON).
		WithRole("measurement").
		WithScope("BTC/USD").
		WithPayload(datura.Map[any]{
			"symbol": "BTC/USD",
			"volume": volume,
			"last":   last,
			"bid":    last - 1,
			"ask":    last + 1,
		}.Marshal())
	frame.SetTimestamp(timestamp)

	return frame
}
