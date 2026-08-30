package types

import (
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/telemetry/generated/telemetry"
)

func TestNewEquityReading(t *testing.T) {
	Convey("Given a complete broker trade balance", t, func() {
		balance := kraken.TradeBalanceResult{
			TradeBalance:  decimal.NewFromFloat64(1000),
			UnrealizedPnL: decimal.NewFromFloat64(-25.5),
			Equity:        decimal.NewFromFloat64(974.5),
		}

		Convey("It should carry cash, unrealized, and equity as decimal strings", func() {
			reading := NewEquityReading(balance)

			So(reading, ShouldNotBeNil)
			So(reading.Cash, ShouldEqual, "1000")
			So(reading.Unrealized, ShouldEqual, "-25.5")
			So(reading.Equity, ShouldEqual, "974.5")
		})
	})

	Convey("Given a balance the broker has not yet valued", t, func() {
		balance := kraken.TradeBalanceResult{
			TradeBalance: decimal.NewFromFloat64(1000),
		}

		Convey("It should yield no reading rather than a reading of zeros", func() {
			So(NewEquityReading(balance), ShouldBeNil)
		})
	})

	Convey("Given a valued account with no open positions", t, func() {
		balance := kraken.TradeBalanceResult{
			TradeBalance: decimal.NewFromFloat64(1000),
			Equity:       decimal.NewFromFloat64(1000),
		}

		Convey("It should report the valuation with an absent unrealized", func() {
			reading := NewEquityReading(balance)

			So(reading, ShouldNotBeNil)
			So(reading.Equity, ShouldEqual, "1000")
			So(reading.Unrealized, ShouldEqual, "")
		})
	})
}

func TestEnvelopeEncode(t *testing.T) {
	Convey("Given an envelope carrying an account valuation", t, func() {
		envelope := &Envelope{
			Key: "BTC/USD",
			Equity: &EquityReading{
				Cash:       "1000",
				Unrealized: "-25.5",
				Equity:     "974.5",
			},
		}

		Convey("It should project the valuation onto the wire state", func() {
			encoded := envelope.Encode()

			So(encoded, ShouldNotBeNil)
			So(encoded.Equity, ShouldNotBeNil)
			So(encoded.Equity.Cash, ShouldEqual, "1000")
			So(encoded.Equity.Unrealized, ShouldEqual, "-25.5")
			So(encoded.Equity.Equity, ShouldEqual, "974.5")
		})

		Convey("It should survive a round trip through the encoded bytes", func() {
			decoded := telemetry.GetRootAsEnvelopeState(envelope.EncodeBytes(), 0)

			So(decoded, ShouldNotBeNil)

			equity := decoded.Equity(nil)
			So(equity, ShouldNotBeNil)
			So(string(equity.Cash()), ShouldEqual, "1000")
			So(string(equity.Unrealized()), ShouldEqual, "-25.5")
			So(string(equity.Equity()), ShouldEqual, "974.5")
		})
	})

	Convey("Given an envelope produced before the first valuation", t, func() {
		envelope := &Envelope{Key: "BTC/USD"}

		Convey("It should leave the wire equity absent rather than zeroed", func() {
			encoded := envelope.Encode()

			So(encoded, ShouldNotBeNil)
			So(encoded.Equity, ShouldBeNil)
		})
	})
}
