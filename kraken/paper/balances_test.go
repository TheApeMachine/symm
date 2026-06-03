package paper

import (
	"context"
	"encoding/json"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/internal/testconfig"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/kraken/user"
)

func TestBalancesSnapshot(t *testing.T) {
	testconfig.Load(t)

	Convey("Given paper balances", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ(ctx, 1, 4, nil)
		ws := NewWebSocket(ctx, pool)
		defer ws.Close()

		balances := NewBalances(ws, NewIdentifier(), NewPairCatalog(ctx))

		Convey("It should seed quote wallet from config", func() {
			message := balances.Send(&qpool.QValue[any]{Value: user.SubscribeFrame{}})

			channel, _ := message["channel"].(string)
			data, _ := message["data"].(json.RawMessage)

			So(channel, ShouldEqual, public.BalancesChannel)
			So(string(data), ShouldContainSubstring, `"asset"`)
		})
	})
}

func TestBalancesApplyFill(t *testing.T) {
	testconfig.Load(t)

	Convey("Given a buy fill", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ(ctx, 1, 4, nil)
		ws := NewWebSocket(ctx, pool)
		defer ws.Close()

		catalog := NewPairCatalog(ctx)
		balances := NewBalances(ws, NewIdentifier(), catalog)

		balances.ApplyFill("BTC/EUR", "buy", 0.01, 50_000, 1, "exec-1")
		after := balances.snapshot()

		Convey("It should credit base asset after a buy", func() {
			channel, _ := after["channel"].(string)
			data, _ := after["data"].(json.RawMessage)

			So(channel, ShouldEqual, public.BalancesChannel)
			So(string(data), ShouldContainSubstring, `"asset":"BTC"`)
		})
	})
}
