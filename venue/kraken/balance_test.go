package kraken_test

import (
	"testing"

	"github.com/bytedance/sonic"
	"github.com/shopspring/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/symm/venue"
	"github.com/theapemachine/symm/venue/kraken"
)

func TestBalanceJSONRoundTrip(t *testing.T) {
	Convey("Given a REST Balance result map", t, func() {
		btcBal, _ := decimal.NewFromString("1.5")
		usdBal, _ := decimal.NewFromString("1000.50")

		original := &kraken.Balance{
			Error: []any{},
			Result: map[string]*decimal.Decimal{
				"XXBT": &btcBal,
				"ZUSD": &usdBal,
			},
		}

		Convey("Unmarshal normalizes REST ledger codes onto desk names", func() {
			raw, err := sonic.Marshal(original)
			So(err, ShouldBeNil)

			var decoded kraken.Balance
			So(sonic.Unmarshal(raw, &decoded), ShouldBeNil)
			So(decoded.QuoteCash("USD").String(), ShouldEqual, usdBal.String())
			So(decoded.QuoteCash("XBT").String(), ShouldEqual, btcBal.String())
		})
	})
}

func TestBalanceToMarketViaVenue(t *testing.T) {
	Convey("Venue Balance Unmarshal fills a market.Balance book", t, func() {
		host := venue.NewKraken("USD")
		book := market.NewBalance(map[string]any{
			"quote":  "USD",
			"assets": []*market.Asset{},
		})
		changed := 0

		host.Balance().OnChange(func(frame market.MarketType) {
			frame.ToMarket(book)
			changed++
		})

		raw := []byte(`{
			"error":[],
			"result":{
				"ZUSD":"200.00",
				"XXBT":"0.5"
			}
		}`)

		So(sonic.Unmarshal(raw, host.Balance()), ShouldBeNil)
		So(changed, ShouldEqual, 1)
		So(book.Quote, ShouldEqual, "USD")
		So(len(book.Assets), ShouldEqual, 2)
		So(book.Cash("USD").String(), ShouldEqual, "200")
		So(book.Cash("XBT").String(), ShouldEqual, "0.5")
	})
}

func TestBalanceWebsocketSnapshotUsesSpotMain(t *testing.T) {
	Convey("A balances snapshot keeps spot main and ignores earn", t, func() {
		host := venue.NewKraken("USD")
		book := market.NewBalance(map[string]any{
			"quote":  "USD",
			"assets": []*market.Asset{},
		})

		host.Balance().OnChange(func(frame market.MarketType) {
			frame.ToMarket(book)
		})

		raw := []byte(`{
			"channel":"balances",
			"type":"snapshot",
			"data":[
				{
					"asset":"USD",
					"balance":1000,
					"wallets":[
						{"type":"spot","id":"main","balance":200},
						{"type":"earn","id":"flex","balance":800}
					]
				},
				{
					"asset":"BTC",
					"balance":1.2,
					"wallets":[{"type":"spot","id":"main","balance":1.2}]
				}
			]
		}`)

		So(sonic.Unmarshal(raw, host.Balance()), ShouldBeNil)
		So(book.Cash("USD").String(), ShouldEqual, "200")
		So(book.Cash("XBT").String(), ShouldEqual, "1.2")

		update := []byte(`{
			"channel":"balances",
			"type":"update",
			"data":[{
				"asset":"USD",
				"wallet_type":"spot",
				"wallet_id":"main",
				"amount":-50,
				"balance":950
			}]
		}`)

		So(sonic.Unmarshal(update, host.Balance()), ShouldBeNil)
		So(book.Cash("USD").String(), ShouldEqual, "150")
		So(book.Cash("XBT").String(), ShouldEqual, "1.2")
	})
}
