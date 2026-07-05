package pumpdump

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
)

func TestTickerMeasure(t *testing.T) {
	Convey("Given the production pumpdump ticker calculator", t, func() {
		ticker := NewTicker()
		timestamp := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)

		for index := range 24 {
			row := tickerRow(
				"BTC/USD",
				1000+float64(index)*10,
				10000+float64(index)*100,
				timestamp.Add(time.Duration(index)*time.Second),
			)

			_, err := ticker.Measure(row, nil)
			So(err, ShouldBeNil)
		}

		spike := tickerRow("BTC/USD", 5000, 20000, timestamp.Add(25*time.Second))

		Convey("When a vertical volume and price spike is measured", func() {
			measured, err := ticker.Measure(spike, nil)

			Convey("Then it should publish classifier output", func() {
				So(err, ShouldBeNil)
				So(measured, ShouldNotBeNil)
				So(len(measured), ShouldEqual, 1)
				So(measured[0].Metrics["ignition"], ShouldBeGreaterThan, 0)
				So(dominantConfidence(measured[0]), ShouldBeGreaterThan, 0.25)
			})
		})
	})
}

func tickerRow(
	symbol string,
	volume float64,
	last float64,
	timestamp time.Time,
) kraken.TickerData {
	return kraken.TickerData{
		Symbol:    symbol,
		Bid:       last - 1,
		Ask:       last + 1,
		Last:      last,
		Volume:    volume,
		Timestamp: timestamp,
	}
}
