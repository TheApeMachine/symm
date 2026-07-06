package pumpdump

import (
	"testing"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	tickerfixture "github.com/theapemachine/symm/tests/fixtures/ticker"
	"github.com/theapemachine/symm/types"
)

func TestTickerMeasure(t *testing.T) {
	Convey("Given the production pumpdump ticker calculator", t, func() {
		ticker := NewTicker()
		fixture := tickerfixture.NewFixture(tickerfixture.UPDATE, 32)

		Convey("When Kraken ticker fixture updates are measured", func() {
			measurements := make([]*types.Measurement, 0)

			for payload := range fixture.Generate() {
				frame := kraken.Ticker{}
				So(sonic.Unmarshal(payload, &frame), ShouldBeNil)
				So(frame.Channel, ShouldEqual, "ticker")
				So(frame.Type, ShouldEqual, "update")

				for _, row := range frame.Data {
					out, err := ticker.Measure(row, nil)
					So(err, ShouldBeNil)
					measurements = append(measurements, out...)
				}
			}

			Convey("Then it should publish classifier output", func() {
				So(measurements, ShouldNotBeEmpty)
				measurement := measurements[len(measurements)-1]
				So(measurement.Source, ShouldEqual, types.SourcePumpDump)
				So(measurement.Symbol, ShouldEqual, "ALGO/USD")
				So(measurement.Metrics["ignition"], ShouldBeGreaterThan, 0)

				confidence := 0.0

				for _, categoryRow := range measurement.Categories {
					if categoryRow.Confidence > confidence {
						confidence = categoryRow.Confidence
					}
				}

				So(confidence, ShouldBeGreaterThan, 0.25)
			})
		})
	})
}
