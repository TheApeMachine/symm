package broker

import (
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
