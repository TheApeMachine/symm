package broker

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/kraken/user"
)

func TestLedgerApplyExecutionDedupesExecID(t *testing.T) {
	Convey("Given a ledger with one execution", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 4, nil)
		ledger := NewLedger(ctx, pool)

		execution := user.Execution{
			ExecID:    "exec-1",
			Symbol:    "BTC/USD",
			Side:      string(trading.Buy),
			LastQty:   0.01,
			LastPrice: 50_000,
		}

		ledger.applyExecution(execution)
		cashAfterFirst := ledger.quoteCash

		Convey("It should ignore duplicate exec_id replays", func() {
			ledger.applyExecution(execution)

			So(ledger.quoteCash, ShouldEqual, cashAfterFirst)
			So(len(ledger.seenExecIDs), ShouldEqual, 1)
		})
	})
}
