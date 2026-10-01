package broker

import (
	"context"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
)

func TestCashPrefersAvailableOverTotal(t *testing.T) {
	Convey("Cash sizes from Available when Total is larger (open holds)", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		balance := NewBalance(ctx, nil)
		balance.Quote = "USD"
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
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		balance := NewBalance(ctx, nil)
		balance.Quote = "USD"
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
