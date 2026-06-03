package paper

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/trading"
)

func TestTradingClientLimitAck(t *testing.T) {
	Convey("Given paper trading with a quoted symbol", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		profilePath := filepath.Join(t.TempDir(), "latency.json")
		viper.Set("trading.model", "paper")
		viper.Set("trading.paper.latency_profile", profilePath)
		viper.Set("trading.order_ack_timeout", 2*time.Second)

		t.Cleanup(viper.Reset)

		pool := qpool.NewQ(ctx, 2, 4, qpool.NewConfig())
		defer pool.Close()

		_ = NewWebSocket(ctx, pool)

		quotes := broker.EnsureQuoteCache(ctx, pool)
		quotes.InstallQuoteForTest(broker.Quote{
			Symbol:    "SYN/EUR",
			Bid:       99.5,
			Ask:       100.5,
			Last:      100,
			UpdatedAt: time.Now().UTC(),
			Book: market.Book{
				Bids: []market.BookLevel{{Price: 100, Qty: 0.0001}},
				Asks: []market.BookLevel{{Price: 100.5, Qty: 10}},
			},
		})

		trading.MarkDeskReady()

		client, err := trading.NewOrder(ctx, pool)
		So(err, ShouldBeNil)

		defer client.Close()

		addErr := client.AddOrder(trading.AddParams{
			OrderType:  trading.Limit,
			Side:       trading.Buy,
			Symbol:     "SYN/EUR",
			OrderQty:   0.01,
			LimitPrice: 100,
			PostOnly:   true,
			ClOrdID:    "paper-trading-ack",
		})

		So(addErr, ShouldBeNil)

		time.Sleep(10 * time.Millisecond)

		bg := pool.CreateBroadcastGroup("kraken:private", 10*time.Millisecond)
		sub := bg.Subscribe("kraken:private", 1024)

		select {
		case message := <-sub.Incoming:
			So(message.Type, ShouldEqual, "update")
			So(message.Value, ShouldNotBeNil)
		case <-time.After(2 * time.Second):
			So("timed out waiting for paper ack", ShouldBeBlank)
		}
	})
}
