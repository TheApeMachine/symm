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

func TestMarshalJSONBalance(t *testing.T) {
	Convey("Setup", t, func() {
		btcBal := decimal.RequireFromString("1.5")
		usdBal := decimal.RequireFromString("1000.50")
		original := &kraken.Balance{
			Error: []any{},
			Result: map[string]*decimal.Decimal{
				"XXBT": &btcBal,
				"ZUSD": &usdBal,
			},
		}

		Convey("When REST Balance round-trips through sonic", func() {
			raw, err := sonic.Marshal(original)
			So(err, ShouldBeNil)

			var decoded kraken.Balance
			So(sonic.Unmarshal(raw, &decoded), ShouldBeNil)

			Convey("Normalize maps ledger codes to desk names", func() {
				So(decoded.QuoteCash("USD").String(), ShouldEqual, "1000.5")
				So(decoded.QuoteCash("XBT").String(), ShouldEqual, "1.5")
			})
		})
	})
}

func TestUnmarshalJSONBalance(t *testing.T) {
	Convey("Setup", t, func() {
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

		Convey("When REST Balance JSON arrives", func() {
			raw := []byte(`{
				"error":[],
				"result":{
					"ZUSD":"200.00",
					"XXBT":"0.5"
				}
			}`)
			So(sonic.Unmarshal(raw, host.Balance()), ShouldBeNil)

			Convey("OnChange fans out normalized cash", func() {
				So(changed, ShouldEqual, 1)
				So(book.Quote, ShouldEqual, "USD")
				So(len(book.Assets), ShouldEqual, 2)
				So(book.Cash("USD").String(), ShouldEqual, "200")
				So(book.Cash("XBT").String(), ShouldEqual, "0.5")
			})
		})

		Convey("When a balances websocket snapshot arrives", func() {
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

			Convey("spot main is spendable and earn is ignored", func() {
				So(book.Cash("USD").String(), ShouldEqual, "200")
				So(book.Cash("XBT").String(), ShouldEqual, "1.2")
			})

			Convey("and updates apply spot deltas", func() {
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
		})
	})
}

func TestToMarketBalance(t *testing.T) {
	Convey("Setup", t, func() {
		equity := decimal.RequireFromString("1250.5")
		unrealized := decimal.RequireFromString("12.25")
		book := market.NewBalance(map[string]any{
			"quote":  "USD",
			"assets": []*market.Asset{},
		})
		book.Equity = &equity
		book.Unrealized = &unrealized
		cash := decimal.RequireFromString("24.62")
		frame := &kraken.Balance{
			Result: map[string]*decimal.Decimal{"USD": &cash},
		}

		Convey("When ToMarket merges cash", func() {
			frame.ToMarket(book)

			Convey("equity and unrealized stay intact", func() {
				So(book.Cash("USD").String(), ShouldEqual, "24.62")
				So(book.Equity.String(), ShouldEqual, "1250.5")
				So(book.Unrealized.String(), ShouldEqual, "12.25")
			})
		})
	})
}

func TestSpotAsset(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When SpotAsset normalizes ledger codes", func() {
			Convey("ZUSD maps to USD and XXBT to XBT", func() {
				So(kraken.SpotAsset("ZUSD"), ShouldEqual, "USD")
				So(kraken.SpotAsset("XXBT"), ShouldEqual, "XBT")
				So(kraken.SpotAsset("USD"), ShouldEqual, "USD")
			})
		})
	})
}

func TestQuoteCash(t *testing.T) {
	Convey("Setup", t, func() {
		usd := decimal.RequireFromString("10")
		balance := &kraken.Balance{
			Result: map[string]*decimal.Decimal{"USD": &usd},
		}

		Convey("When QuoteCash reads a positive balance", func() {
			cash := balance.QuoteCash("USD")

			Convey("the amount is returned", func() {
				So(cash.String(), ShouldEqual, "10")
			})
		})

		Convey("When the asset is missing", func() {
			cash := balance.QuoteCash("EUR")

			Convey("nil is returned", func() {
				So(cash, ShouldBeNil)
			})
		})
	})
}

func BenchmarkMarshalJSONBalance(b *testing.B) {
	btcBal := decimal.RequireFromString("1.5")
	usdBal := decimal.RequireFromString("1000.50")
	original := &kraken.Balance{
		Error: []any{},
		Result: map[string]*decimal.Decimal{
			"XXBT": &btcBal,
			"ZUSD": &usdBal,
		},
	}

	b.ReportAllocs()

	for b.Loop() {
		raw, err := sonic.Marshal(original)
		if err != nil {
			b.Fatal(err)
		}
		var decoded kraken.Balance
		_ = sonic.Unmarshal(raw, &decoded)
		_ = decoded.QuoteCash("USD")
	}
}

func BenchmarkUnmarshalJSONBalance(b *testing.B) {
	host := venue.NewKraken("USD")
	raw := []byte(`{
		"error":[],
		"result":{
			"ZUSD":"200.00",
			"XXBT":"0.5"
		}
	}`)

	b.ReportAllocs()

	for b.Loop() {
		_ = sonic.Unmarshal(raw, host.Balance())
	}
}

func BenchmarkToMarketBalance(b *testing.B) {
	equity := decimal.RequireFromString("1250.5")
	unrealized := decimal.RequireFromString("12.25")
	book := market.NewBalance(map[string]any{
		"quote":  "USD",
		"assets": []*market.Asset{},
	})
	book.Equity = &equity
	book.Unrealized = &unrealized
	cash := decimal.RequireFromString("24.62")
	frame := &kraken.Balance{
		Result: map[string]*decimal.Decimal{"USD": &cash},
	}

	b.ReportAllocs()

	for b.Loop() {
		frame.ToMarket(book)
	}
}

func BenchmarkSpotAsset(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		_ = kraken.SpotAsset("ZUSD")
		_ = kraken.SpotAsset("XXBT")
	}
}

func BenchmarkQuoteCash(b *testing.B) {
	usd := decimal.RequireFromString("10")
	balance := &kraken.Balance{
		Result: map[string]*decimal.Decimal{"USD": &usd},
	}

	b.ReportAllocs()

	for b.Loop() {
		_ = balance.QuoteCash("USD")
	}
}
