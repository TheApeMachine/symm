package broker

import (
	"sync"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
TestMarksApplySkipsClosedLots ensures closed shells are not re-marked or
republished after the wallet has flattened them.
*/
func TestMarksApplySkipsClosedLots(t *testing.T) {
	Convey("Given a CLOSED holding still registered on the marks map", t, func() {
		messages := make(chan []byte, 1)
		holding := &types.Holding{
			Symbol:     "ETH/USD",
			Asset:      "ETH",
			Qty:        decimal.NewFromFloat64(0),
			EntryPrice: decimal.NewFromFloat64(1859.86),
			Mark:       decimal.NewFromFloat64(1858.0),
			Status:     types.CLOSED,
		}
		holdings := &sync.Map{}
		holdings.Store("ETH/USD", holding)
		cash := decimal.NewFromFloat64(196.76)
		balance := &Balance{
			status:   types.READY,
			quote:    "USD",
			ui:       messages,
			holdings: holdings,
			model: &kraken.Balance{Data: []kraken.BalanceData{{
				Asset: "USD", Balance: cash, Available: cash,
			}}},
		}
		positions := &sync.Map{}
		positions.Store("ETH/USD", &Position{
			balance: balance,
			price:   NewPrice(nil),
			pair: &kraken.InstrumentPair{
				Symbol: "ETH/USD", Base: "ETH", Quote: "USD",
			},
		})
		marks := &Marks{positions: positions}
		prior := holding.Mark.Copy()

		marks.Apply("ETH/USD")

		Convey("Then mark and publish stay quiet", func() {
			So(holding.Mark.Cmp(prior), ShouldEqual, 0)
			So(len(messages), ShouldEqual, 0)
		})
	})
}
