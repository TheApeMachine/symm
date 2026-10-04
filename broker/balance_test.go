package broker

import (
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
)

func TestCashPrefersAvailableOverTotal(t *testing.T) {
	Convey("Cash sizes from Available when Total is larger (open holds)", t, func() {
		balance := &Balance{Quote: "USD"}
		balance.UpdateWallet(&kraken.Balance{
			Data: []kraken.BalanceData{{
				Asset:     "USD",
				Balance:   decimal.NewFromFloat64(1000),
				Available: decimal.NewFromFloat64(0.19331402),
				Reserved:  decimal.NewFromFloat64(999.80668598),
			}},
		})

		cash := balance.Cash()
		So(cash, ShouldNotBeNil)
		So(cash.Cmp(decimal.NewFromFloat64(0.19331402)), ShouldEqual, 0)
	})
}

func TestCashFallsBackToBalanceWhenAvailableAbsent(t *testing.T) {
	Convey("Cash uses Balance when Available is nil", t, func() {
		balance := &Balance{Quote: "USD"}
		balance.UpdateWallet(&kraken.Balance{
			Data: []kraken.BalanceData{{
				Asset:   "USD",
				Balance: decimal.NewFromFloat64(42),
			}},
		})

		cash := balance.Cash()
		So(cash, ShouldNotBeNil)
		So(cash.Cmp(decimal.NewFromFloat64(42)), ShouldEqual, 0)
	})
}

func TestBalance_UpdateWallet(t *testing.T) {
	Convey("Given a balance holding the venue's last trade balance", t, func() {
		balance := &Balance{Quote: "USD"}
		balance.snapshot.Store(newAccountSnapshot("USD", nil, &kraken.TradeBalanceResult{
			Equity:        decimal.NewFromFloat64(199.16),
			UnrealizedPnL: decimal.NewFromFloat64(-0.84),
		}))

		Convey("When a streamed wallet frame arrives", func() {
			balance.UpdateWallet(&kraken.Balance{
				Data: []kraken.BalanceData{{
					Asset:     "USD",
					Balance:   decimal.NewFromFloat64(150),
					Available: decimal.NewFromFloat64(150),
				}},
			})

			Convey("It updates cash and keeps the venue's equity and unrealized PnL", func() {
				frame := balance.EquityWire()
				So(frame, ShouldNotBeNil)
				So(frame.Cash, ShouldEqual, decimal.NewFromFloat64(150).String())
				So(frame.Equity, ShouldEqual, decimal.NewFromFloat64(199.16).String())
				So(frame.Unrealized, ShouldEqual, decimal.NewFromFloat64(-0.84).String())
			})
		})
	})
}
