package paper

import (
	"context"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/kraken/trading"
)

func TestOrdersMatchRestingLimit(t *testing.T) {
	convey.Convey("Given a resting post-only buy", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		pool := qpool.NewQ(ctx, 1, 4, nil)
		ws := NewWebSocket(ctx, pool)
		defer ws.Close()

		orders := ws.sockets[public.OrdersChannel].(*Orders)
		quotes := broker.EnsureQuoteCache(ctx, pool)

		quotes.InstallQuoteForTest(broker.Quote{
			Symbol:    "BTC/EUR",
			Bid:       49_900,
			Ask:       50_500,
			Last:      50_000,
			UpdatedAt: time.Now().UTC(),
		})

		open := orders.Send(&qpool.QValue[any]{
			Type: public.OrdersChannel,
			Value: map[string]any{
				"method": trading.MethodAddOrder,
				"params": trading.AddParams{
					OrderType:  trading.Limit,
					Side:       trading.Buy,
					Symbol:     "BTC/EUR",
					OrderQty:   0.01,
					LimitPrice: 50_000,
					PostOnly:   true,
					ClOrdID:    "resting-buy",
				},
			},
		})

		convey.Convey("It should fill when the ask crosses the limit", func() {
			convey.So(string(open.Data), convey.ShouldContainSubstring, `"order_status":"open"`)

			crossed := broker.Quote{
				Symbol:    "BTC/EUR",
				Bid:       49_900,
				Ask:       49_950,
				Last:      49_925,
				UpdatedAt: time.Now().UTC(),
			}

			orders.tryMatchQuote("BTC/EUR", crossed)

			_, stillOpen := orders.orderByClOrdID("resting-buy")
			convey.So(stillOpen, convey.ShouldBeFalse)
		})
	})
}
