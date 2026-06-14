package integration

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/broker"
)

func TestMaster(t *testing.T) {
	Convey("Given a trading system", t, func() {
		ctx := t.Context()
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		pool := qpool.NewQ[any](ctx, 1, 1, nil)
		defer pool.Close()

		trader := broker.NewTrader(ctx)
		So(trader, ShouldNotBeNil)
	})
}
