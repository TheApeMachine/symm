package kraken_test

import (
	"testing"

	"github.com/bytedance/sonic"
	"github.com/shopspring/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/venue/kraken"
)

func TestTradeBalanceRESTUnmarshal(t *testing.T) {
	Convey("REST TradeBalance decodes Kraken e and n fields", t, func() {
		raw := []byte(`{
			"error":[],
			"result":{
				"eb":"140.9725",
				"tb":"141.5225",
				"m":"0.0000",
				"n":"-1.5500",
				"c":"72.0900",
				"v":"70.5400",
				"e":"139.9725",
				"mf":"139.9725"
			}
		}`)

		tb := kraken.NewTradeBalance()

		So(tb.UnmarshalJSON(raw), ShouldBeNil)
		So(tb.Result.Equity.String(), ShouldEqual, "139.9725")
		So(tb.Result.UnrealizedPnL.String(), ShouldEqual, "-1.55")
	})
}

func TestTradeBalanceResultMeasurementRoundTrip(t *testing.T) {
	Convey("Given a valid kraken.TradeBalanceResult and wallet", t, func() {
		unrealized, _ := decimal.NewFromString("-0.32")
		equity, _ := decimal.NewFromString("199.68")
		cash, _ := decimal.NewFromString("160.14")
		zamaAmount, _ := decimal.NewFromString("517.71")
		unrealizedPtr := &unrealized
		equityPtr := &equity
		cashPtr := &cash
		zamaPtr := &zamaAmount

		tb := &kraken.TradeBalanceResult{
			UnrealizedPnL: unrealizedPtr,
			Equity:        equityPtr,
			AvailableCash: cashPtr,
		}

		wallet := &kraken.Balance{
			Error: []any{},
			Result: map[string]*decimal.Decimal{
				"USD":  cashPtr,
				"ZAMA": zamaPtr,
			},
		}

		Convey("sonic.Marshal round-trips the trade balance and wallet", func() {
			balanceRaw, err := sonic.Marshal(tb)
			So(err, ShouldBeNil)

			var decoded kraken.TradeBalanceResult
			So(sonic.Unmarshal(balanceRaw, &decoded), ShouldBeNil)
			So(decoded.Equity.String(), ShouldEqual, equity.String())
			So(decoded.UnrealizedPnL.String(), ShouldEqual, unrealized.String())
			So(decoded.AvailableCash.String(), ShouldEqual, cash.String())

			walletRaw, err := sonic.Marshal(wallet)
			So(err, ShouldBeNil)

			var decodedWallet kraken.Balance
			So(sonic.Unmarshal(walletRaw, &decodedWallet), ShouldBeNil)
			So(decodedWallet.Result["USD"].String(), ShouldEqual, cash.String())
			So(decodedWallet.Result["ZAMA"].String(), ShouldEqual, zamaAmount.String())
		})
	})
}
