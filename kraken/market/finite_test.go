package market

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSymbolValidateZeroPressure(t *testing.T) {
	Convey("Given a symbol row with balanced trade flow", t, func() {
		row := &Symbol{
			Name:     "ZBCN/USD",
			Price:    100,
			Value:    0.01,
			Volume:   1000,
			Pressure: 0,
			Updated:  time.Now(),
		}

		Convey("It should validate with zero pressure", func() {
			So(row.Validate(), ShouldBeNil)
		})
	})
}

func TestNewSymbolRowZeroPressure(t *testing.T) {
	Convey("Given zero trade pressure", t, func() {
		row, err := NewSymbolRow("ZBCN/USD", 100, 0.01, 1000, 0, time.Now())

		Convey("It should build a valid row", func() {
			So(err, ShouldBeNil)
			So(row, ShouldNotBeNil)
			So(row.Validate(), ShouldBeNil)
			So(row.Pressure, ShouldEqual, 0)
		})
	})
}

func TestTickerUpdateValidate(t *testing.T) {
	Convey("Given a ticker row with book prices but no 24h high", t, func() {
		ticker := &TickerUpdate{
			Symbol: "ILLQ/EUR",
			Bid:    1.0,
			Ask:    1.1,
		}

		Convey("It should validate when a price is resolvable", func() {
			So(ticker.Validate(), ShouldBeNil)
		})
	})

	Convey("Given a ticker row with only last price", t, func() {
		ticker := &TickerUpdate{
			Symbol: "BTC/EUR",
			Last:   50000,
		}

		Convey("It should validate", func() {
			So(ticker.Validate(), ShouldBeNil)
		})
	})

	Convey("Given a ticker row without any price", t, func() {
		ticker := &TickerUpdate{Symbol: "BTC/EUR"}

		Convey("It should reject the row", func() {
			So(ticker.Validate(), ShouldNotBeNil)
		})
	})

	Convey("Given a ticker row with inverted touch", t, func() {
		ticker := &TickerUpdate{
			Symbol: "BTC/EUR",
			Bid:    101,
			Ask:    100,
		}

		Convey("It should reject the row", func() {
			So(ticker.Validate(), ShouldNotBeNil)
		})
	})
}

func BenchmarkTickerUpdateValidate(b *testing.B) {
	ticker := &TickerUpdate{
		Symbol:    "BTC/EUR",
		Bid:       49990,
		Ask:       50010,
		Last:      50000,
		ChangePct: 0.02,
	}

	for b.Loop() {
		_ = ticker.Validate()
	}
}
