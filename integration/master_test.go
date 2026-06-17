package integration

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/logic"
)

func TestMaster(testingTB *testing.T) {
	Convey("Given a trading system", testingTB, func() {
		ctx := testingTB.Context()
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		pool := qpool.NewQ[any](ctx, 2, 4, &qpool.Config{Scaler: nil})
		trader := broker.NewTrader(ctx)
		tree, treeErr := logic.NewTree(ctx, pool)

		Convey("It should wire trader and playbook tree", func() {
			So(trader, ShouldNotBeNil)
			So(treeErr, ShouldBeNil)
			So(tree, ShouldNotBeNil)
			So(len(tree.Branches), ShouldBeGreaterThan, 0)
		})
	})
}
