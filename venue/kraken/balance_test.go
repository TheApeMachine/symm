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

		Convey("sonic.Marshal round-trips result assets", func() {
			raw, err := sonic.Marshal(original)
			So(err, ShouldBeNil)

			var decoded kraken.Balance
			So(sonic.Unmarshal(raw, &decoded), ShouldBeNil)
			So(decoded.Result["XXBT"].String(), ShouldEqual, btcBal.String())
			So(decoded.Result["ZUSD"].String(), ShouldEqual, usdBal.String())
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
		So(book.Cash("ZUSD").String(), ShouldEqual, "200")
		So(book.Cash("XXBT").String(), ShouldEqual, "0.5")
	})
}
