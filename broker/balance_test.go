package broker

import (
	"github.com/theapemachine/symm/kraken"
	venue "github.com/theapemachine/symm/tests/venue"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

func TestBalanceUpdate(t *testing.T) {
	Convey("Given Kraken's legacy REST asset names", t, func() {
		viper.Set("market.quote_currency", "USD")
		defer viper.Reset()

		conn := venue.NewConn()
		conn.BalanceResult = map[string]*decimal.Decimal{
			"ZUSD": venue.Decimal("200.00"),
			"XXBT": venue.Decimal("0.001"),
		}
		api := websocket.NewAPI(t.Context(), conn, conn)
		api.Normalizer().Update(&spot.AssetsManagerUpdate{
			NewAssets: map[string]spot.AssetInfo{
				"USD": {AltName: "USD"},
				"BTC": {AltName: "XBT"},
			},
			OldAssets: map[string]spot.AssetInfo{
				"ZUSD": {AltName: "USD"},
				"XXBT": {AltName: "XBT"},
			},
		})

		balance := NewBalance(api)

		Convey("the wallet stores canonical names and exposes quote cash", func() {
			assets := balance.Assets()

			So(balance.Status(), ShouldEqual, types.READY)
			So(balance.Cash().Cmp(venue.Decimal("200.00")), ShouldEqual, 0)
			So(assets["BTC"].Cmp(venue.Decimal("0.001")), ShouldEqual, 0)
			So(assets, ShouldNotContainKey, "ZUSD")
			So(assets, ShouldNotContainKey, "XXBT")
		})
	})
}

func BenchmarkBalanceUpdate(b *testing.B) {
	viper.Set("market.quote_currency", "USD")
	defer viper.Reset()

	conn := venue.NewConn()
	conn.BalanceResult = map[string]*decimal.Decimal{
		"ZUSD": decimal.NewFromInt64(200),
		"XXBT": decimal.NewFromFloat64(0.001),
	}
	api := websocket.NewAPI(b.Context(), conn, conn)
	api.Normalizer().Update(&spot.AssetsManagerUpdate{
		NewAssets: map[string]spot.AssetInfo{
			"USD": {AltName: "USD"},
			"BTC": {AltName: "XBT"},
		},
		OldAssets: map[string]spot.AssetInfo{
			"ZUSD": {AltName: "USD"},
			"XXBT": {AltName: "XBT"},
		},
	})
	balance := NewBalance(api)

	for b.Loop() {
		balance.Update()
	}
}

func TestBalanceSettle(t *testing.T) {
	Convey("Given independent funded balances", t, func() {
		account, err := NewFundedBalance("USD", venue.Decimal("200"))
		So(err, ShouldBeNil)
		other, err := NewFundedBalance("USD", venue.Decimal("200"))
		So(err, ShouldBeNil)
		Convey("Complete buys and sells retain exact costs and fees", func() {
			So(account.Settle(kraken.ExecutionData{Side: "buy", CumCost: venue.Decimal("100.00"), FeeUsdEquiv: venue.Decimal("0.25")}), ShouldBeNil)
			So(account.Cash().Cmp(venue.Decimal("99.75")), ShouldEqual, 0)
			So(account.Settle(kraken.ExecutionData{Side: "sell", CumCost: venue.Decimal("110.00"), FeeUsdEquiv: venue.Decimal("0.275")}), ShouldBeNil)
			So(account.Cash().Cmp(venue.Decimal("209.475")), ShouldEqual, 0)
			So(other.Cash().String(), ShouldEqual, "200")
		})
		Convey("Insufficient funds and incomplete fills leave the balance intact", func() {
			So(account.Settle(kraken.ExecutionData{Side: "buy", CumCost: venue.Decimal("200"), FeeUsdEquiv: venue.Decimal("1")}), ShouldNotBeNil)
			So(account.Settle(kraken.ExecutionData{Side: "buy"}), ShouldNotBeNil)
			So(account.Cash().String(), ShouldEqual, "200")
		})
	})
}

func BenchmarkBalanceSettle(b *testing.B) {
	// Each round trip spends two cents in fees. Funding covers every iteration.
	balance, err := NewFundedBalance("USD", decimal.NewFromInt64(int64(b.N)*2+1))

	if err != nil {
		b.Fatal(err)
	}
	fill := kraken.ExecutionData{CumCost: venue.Decimal("1"), FeeUsdEquiv: venue.Decimal("0.01")}
	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		fill.Side = "buy"

		if err := balance.Settle(fill); err != nil {
			b.Fatal(err)
		}
		fill.Side = "sell"

		if err := balance.Settle(fill); err != nil {
			b.Fatal(err)
		}
	}
}
