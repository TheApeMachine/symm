package response

import (
	"math/big"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
)

func exactSum(values []float64) *big.Rat {
	sum := new(big.Rat)

	for _, value := range values {
		sum.Add(sum, new(big.Rat).SetFloat64(value))
	}

	return sum
}

// The ledger of record is exact: a full sell of everything bought leaves the
// base balance at EXACT zero, however many fills built the position. The float
// ledger stranded dust here, which is how unsellable sub-minimum positions and
// stale cost bases were born.
func TestWalletLedgerIsExact(t *testing.T) {
	Convey("Given a funded paper wallet", t, func() {
		viper.Set("market.quote_currency", "EUR")
		viper.Set("trading.paper.wallet_eur", 200.0)

		balances := NewBalances(nil, nil, NewIdentifier())

		Convey("Buy in three odd lots, sell everything in one fill", func() {
			lots := []float64{6666.66667, 6666.66667, 6666.66666}
			total := 0.0

			for _, lot := range lots {
				_, err := balances.ApplyFill("YALA/EUR", "buy", lot, 0.0025, 0.04, "r1")
				So(err, ShouldBeNil)
				total += lot
			}

			// The wallet must hold the exact sum of the fills, not its float echo.
			So(balances.availableRat("YALA").Cmp(exactSum(lots)), ShouldEqual, 0)

			_, err := balances.ApplyFill("YALA/EUR", "sell", total, 0.0026, 0.05, "r2")
			So(err, ShouldBeNil)

			Convey("The base balance is exact zero and the basis is dropped", func() {
				So(balances.availableRat("YALA").Sign(), ShouldEqual, 0)
				_, stale := balances.costBasis["YALA"]
				So(stale, ShouldBeFalse)
			})
		})
	})
}
