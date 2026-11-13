package kraken

import (
	"testing"

	"github.com/bytedance/sonic"
	"github.com/shopspring/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/market"
)

const tradeBalanceREST = `{
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
}`

func TestUnmarshalJSONTradeBalance(t *testing.T) {
	Convey("Setup", t, func() {
		tb := NewTradeBalance()

		Convey("When REST TradeBalance JSON arrives", func() {
			So(tb.UnmarshalJSON([]byte(tradeBalanceREST)), ShouldBeNil)

			Convey("equity and unrealized decode from e and n", func() {
				So(tb.Result.Equity.String(), ShouldEqual, "139.9725")
				So(tb.Result.UnrealizedPnL.String(), ShouldEqual, "-1.55")
			})
		})
	})
}

func TestToMarketTradeBalance(t *testing.T) {
	Convey("Setup", t, func() {
		tb := NewTradeBalance()
		raw := []byte(`{
			"error":[],
			"result":{
				"eb":"161.01",
				"tb":"50.00",
				"m":"0.0000",
				"n":"0.80",
				"c":"72.09",
				"v":"60.60",
				"e":"61.76",
				"mf":"61.76"
			}
		}`)
		So(tb.UnmarshalJSON(raw), ShouldBeNil)

		book := market.NewBalance(map[string]any{
			"quote":  "USD",
			"assets": []*market.Asset{},
		})

		Convey("When ToMarket projects trade balance", func() {
			tb.ToMarket(book)

			Convey("eb drives equity and v-c drives unrealized", func() {
				So(book.Equity.String(), ShouldEqual, "161.01")
				So(book.Unrealized.String(), ShouldEqual, "-11.49")
			})
		})
	})
}

func TestMarshalJSONTradeBalanceResult(t *testing.T) {
	Convey("Setup", t, func() {
		unrealized := decimal.RequireFromString("-0.32")
		equity := decimal.RequireFromString("199.68")
		cash := decimal.RequireFromString("160.14")
		zamaAmount := decimal.RequireFromString("517.71")

		tb := &TradeBalanceResult{
			UnrealizedPnL: &unrealized,
			Equity:        &equity,
			AvailableCash: &cash,
		}
		wallet := &Balance{
			Error: []any{},
			Result: map[string]*decimal.Decimal{
				"USD":  &cash,
				"ZAMA": &zamaAmount,
			},
		}

		Convey("When TradeBalanceResult round-trips", func() {
			balanceRaw, err := sonic.Marshal(tb)
			So(err, ShouldBeNil)

			var decoded TradeBalanceResult
			So(sonic.Unmarshal(balanceRaw, &decoded), ShouldBeNil)

			Convey("decimal fields survive", func() {
				So(decoded.Equity.String(), ShouldEqual, "199.68")
				So(decoded.UnrealizedPnL.String(), ShouldEqual, "-0.32")
				So(decoded.AvailableCash.String(), ShouldEqual, "160.14")
			})

			Convey("and the wallet map round-trips", func() {
				walletRaw, err := sonic.Marshal(wallet)
				So(err, ShouldBeNil)

				var decodedWallet Balance
				So(sonic.Unmarshal(walletRaw, &decodedWallet), ShouldBeNil)
				So(decodedWallet.Result["USD"].String(), ShouldEqual, "160.14")
				So(decodedWallet.Result["ZAMA"].String(), ShouldEqual, "517.71")
			})
		})
	})
}
