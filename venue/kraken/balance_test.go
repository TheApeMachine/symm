package kraken_test

import (
	"testing"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/shopspring/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/symm/venue"
	"github.com/theapemachine/symm/venue/kraken"
)

/*
seedLedgerNames mirrors Assets/AssetPairs aliases the booter loads — old REST
ledger codes collapse onto the desk asset names.
*/
func seedLedgerNames() *spot.Normalizer {
	names := spot.NewNormalizer()
	names.Update(&spot.AssetsManagerUpdate{
		NewAssets: map[string]spot.AssetInfo{
			"USD": {AltName: "USD"},
			"XBT": {AltName: "BTC"},
			"XRP": {AltName: "XRP"},
			"ZEC": {AltName: "ZEC"},
		},
		OldAssets: map[string]spot.AssetInfo{
			"ZUSD": {AltName: "USD"},
			"XXBT": {AltName: "BTC"},
			"XXRP": {AltName: "XRP"},
			"XZEC": {AltName: "ZEC"},
		},
	})
	return names
}

func TestMarshalJSONBalance(t *testing.T) {
	Convey("Setup", t, func() {
		btcBal := decimal.RequireFromString("1.5")
		usdBal := decimal.RequireFromString("1000.50")
		original := kraken.NewBalance().Use(seedLedgerNames())
		original.Error = []any{}
		original.Result = map[string]*decimal.Decimal{
			"XXBT": &btcBal,
			"ZUSD": &usdBal,
		}

		Convey("When REST Balance round-trips through sonic", func() {
			raw, err := sonic.Marshal(original)
			So(err, ShouldBeNil)

			decoded := kraken.NewBalance().Use(seedLedgerNames())
			So(sonic.Unmarshal(raw, decoded), ShouldBeNil)

			Convey("Normalize maps ledger codes to desk names", func() {
				So(decoded.QuoteCash("USD").String(), ShouldEqual, "1000.5")
				So(decoded.QuoteCash("XBT").String(), ShouldEqual, "1.5")
			})
		})
	})
}

func TestUnmarshalJSONBalance(t *testing.T) {
	Convey("Setup", t, func() {
		host := venue.NewKraken("USD").Use(seedLedgerNames())
		book := market.NewBalance(map[string]any{
			"quote":  "USD",
			"assets": []*market.Asset{},
		})
		Convey("When REST Balance JSON arrives", func() {
			raw := []byte(`{
				"error":[],
				"result":{
					"ZUSD":"200.00",
					"XXBT":"0.5",
					"XXRP":"10.06",
					"XZEC":"0.004"
				}
			}`)
			So(sonic.Unmarshal(raw, host.Balance()), ShouldBeNil)
			host.Balance().ToMarket(book)

			Convey("ToMarket fans out normalized cash", func() {
				So(book.Quote, ShouldEqual, "USD")
				So(len(book.Assets), ShouldEqual, 4)
				So(book.Cash("USD").String(), ShouldEqual, "200")
				So(book.Cash("XBT").String(), ShouldEqual, "0.5")
				So(book.Cash("XRP").String(), ShouldEqual, "10.06")
				So(book.Cash("ZEC").String(), ShouldEqual, "0.004")
			})
		})

		Convey("When a balances websocket snapshot arrives", func() {
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
			host.Balance().ToMarket(book)

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
				host.Balance().ToMarket(book)
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
		frame := kraken.NewBalance().Use(seedLedgerNames())
		frame.Result = map[string]*decimal.Decimal{"USD": &cash}

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

func TestBalanceUse(t *testing.T) {
	Convey("Setup", t, func() {
		names := seedLedgerNames()
		balance := kraken.NewBalance().Use(names)
		xxrp := decimal.RequireFromString("10.06")
		xzec := decimal.RequireFromString("0.004")
		balance.Result = map[string]*decimal.Decimal{
			"XXRP": &xxrp,
			"XZEC": &xzec,
		}

		Convey("When Normalize runs with the desk normalizer", func() {
			balance.Normalize()

			Convey("XXRP and XZEC land on pair-base names", func() {
				So(balance.QuoteCash("XRP").String(), ShouldEqual, "10.06")
				So(balance.QuoteCash("ZEC").String(), ShouldEqual, "0.004")
				So(balance.QuoteCash("XXRP").String(), ShouldEqual, "10.06")
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
	original := kraken.NewBalance().Use(seedLedgerNames())
	original.Error = []any{}
	original.Result = map[string]*decimal.Decimal{
		"XXBT": &btcBal,
		"ZUSD": &usdBal,
	}

	b.ReportAllocs()

	for b.Loop() {
		raw, err := sonic.Marshal(original)
		if err != nil {
			b.Fatal(err)
		}
		decoded := kraken.NewBalance().Use(seedLedgerNames())
		_ = sonic.Unmarshal(raw, decoded)
		_ = decoded.QuoteCash("USD")
	}
}

func BenchmarkUnmarshalJSONBalance(b *testing.B) {
	host := venue.NewKraken("USD").Use(seedLedgerNames())
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
	frame := kraken.NewBalance().Use(seedLedgerNames())
	frame.Result = map[string]*decimal.Decimal{"USD": &cash}

	b.ReportAllocs()

	for b.Loop() {
		frame.ToMarket(book)
	}
}

func BenchmarkBalanceNormalize(b *testing.B) {
	xxrp := decimal.RequireFromString("10")
	balance := kraken.NewBalance().Use(seedLedgerNames())
	balance.Result = map[string]*decimal.Decimal{"XXRP": &xxrp}

	b.ReportAllocs()

	for b.Loop() {
		balance.Result = map[string]*decimal.Decimal{"XXRP": &xxrp}
		balance.Normalize()
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
