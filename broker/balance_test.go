package broker

import (
	"github.com/theapemachine/symm/network"
	"testing"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/runtime"
	venue "github.com/theapemachine/symm/tests/venue"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
)

func TestBalanceUpdate(t *testing.T) {
	Convey("Given Kraken's legacy REST asset names", t, func() {
		viper.Set("market.quote_currency", "USD")
		defer viper.Reset()

		conn := venue.NewConn()
		conn.BalanceResult = kraken.NewBalanceFromMap(map[string]*decimal.Decimal{
			"ZUSD": venue.Decimal("200.00"),
			"XXBT": venue.Decimal("0.001"),
		})
		api := network.NewWebsocketClient(t.Context())
		
		balance := NewBalance(t.Context(), api)

		Convey("the wallet stores canonical names and exposes quote cash", func() {
			assets := balance.Assets()

			So(balance.Status(), ShouldEqual, runtime.READY)
			So(balance.Cash().Cmp(venue.Decimal("200.00")), ShouldEqual, 0)
			So(assets["BTC"].Cmp(venue.Decimal("0.001")), ShouldEqual, 0)
			So(assets, ShouldNotContainKey, "ZUSD")
			So(assets, ShouldNotContainKey, "XXBT")
		})
	})
}

func TestBalanceAtomicSnapshot(t *testing.T) {
	Convey("Given a balance manager with simultaneous wallet and trade balance updates", t, func() {
		viper.Set("market.quote_currency", "USD")
		defer viper.Reset()

		conn := venue.NewConn()
		conn.BalanceResult = kraken.NewBalanceFromMap(map[string]*decimal.Decimal{
			"USD": venue.Decimal("1000.00"),
			"BTC": venue.Decimal("0.5"),
		})
		conn.TradeBalanceResult = &kraken.TradeBalanceResult{
			Equity:        venue.Decimal("1500.00"),
			UnrealizedPnL: venue.Decimal("500.00"),
		}

		api := network.NewWebsocketClient(t.Context())
		balance := NewBalance(t.Context(), api)

		Convey("Then Snapshot returns fully synchronized atomic state", func() {
			snap := balance.Snapshot()
			So(snap, ShouldNotBeNil)
			So(snap.Cash.Cmp(venue.Decimal("1000.00")), ShouldEqual, 0)
			So(snap.Equity.Cmp(venue.Decimal("1500.00")), ShouldEqual, 0)
			So(snap.Unrealized.Cmp(venue.Decimal("500.00")), ShouldEqual, 0)
			So(snap.Assets["BTC"].Cmp(venue.Decimal("0.5")), ShouldEqual, 0)

			So(balance.Cash().Cmp(venue.Decimal("1000.00")), ShouldEqual, 0)
			So(balance.Equity().Cmp(venue.Decimal("1500.00")), ShouldEqual, 0)
			So(balance.Unrealized().Cmp(venue.Decimal("500.00")), ShouldEqual, 0)
		})

		Convey("When new data arrives and Refresh is called", func() {
			conn.BalanceResult = kraken.NewBalanceFromMap(map[string]*decimal.Decimal{
				"USD": venue.Decimal("2000.00"),
				"BTC": venue.Decimal("1.0"),
			})
			conn.TradeBalanceResult = &kraken.TradeBalanceResult{
				Equity:        venue.Decimal("3000.00"),
				UnrealizedPnL: venue.Decimal("1000.00"),
			}

			balance.Update()
			err := error(nil)
			So(err, ShouldBeNil)

			snap := balance.Snapshot()
			So(snap, ShouldNotBeNil)
			So(snap.Cash.Cmp(venue.Decimal("2000.00")), ShouldEqual, 0)
			So(snap.Equity.Cmp(venue.Decimal("3000.00")), ShouldEqual, 0)
			So(snap.Unrealized.Cmp(venue.Decimal("1000.00")), ShouldEqual, 0)
			So(snap.Assets["BTC"].Cmp(venue.Decimal("1.0")), ShouldEqual, 0)
		})
	})
}

func BenchmarkBalanceUpdate(b *testing.B) {
	viper.Set("market.quote_currency", "USD")
	defer viper.Reset()

	conn := venue.NewConn()
	conn.BalanceResult = kraken.NewBalanceFromMap(map[string]*decimal.Decimal{
		"ZUSD": decimal.NewFromInt64(200),
		"XXBT": decimal.NewFromFloat64(0.001),
	})
	api := network.NewWebsocketClient(b.Context())
		balance := NewBalance(b.Context(), api)

	for b.Loop() {
		balance.Update()
	}
}
