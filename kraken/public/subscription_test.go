package public

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/qpool"
)

func TestSubscriptionEnsure(testingTB *testing.T) {
	Convey("Given a subscription waiting for market data", testingTB, func() {
		ctx, cancel := context.WithCancel(context.Background())

		defer cancel()

		viper.Set("market.default_symbols", []string{"BTC/USD", "ETH/USD"})
		viper.Set("market.subscribe_batch", 2)
		viper.Set("market.subscribe_pace", 0)

		pool := qpool.NewQ[any](ctx, 1, 2, nil)
		tree := dmt.NewTree(testingTB.TempDir())
		subscription := NewSubscription(ctx, pool, tree)

		received := make(chan *datura.Artifact, 8)
		pool.Subscribe("kraken:public", func(artifact *datura.Artifact) error {
			received <- artifact
			return nil
		})

		err := subscription.Ensure()

		Convey("It should publish instrument and channel subscribe frames", func() {
			So(err, ShouldBeNil)
			So(subscription.Armed(), ShouldBeTrue)

			destinations := make([]string, 0, 4)

			for range 4 {
				artifact := <-received
				destination, destinationErr := artifact.Destination()

				So(destinationErr, ShouldBeNil)
				destinations = append(destinations, destination)
			}

			So(destinations, ShouldContain, "kraken:public")
		})
	})
}

func TestSubscriptionSymbols(testingTB *testing.T) {
	Convey("Given anchor and default symbols", testingTB, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 2, nil)
		tree := dmt.NewTree(testingTB.TempDir())

		viper.Set("market.anchor_symbol", "BTC/USD")
		viper.Set("market.default_symbols", []string{"BTC/USD", "ETH/USD"})

		subscription := NewSubscription(ctx, pool, tree)

		Convey("It should deduplicate while preserving anchor first", func() {
			So(subscription.Symbols(), ShouldResemble, []string{"BTC/USD", "ETH/USD"})
		})
	})
}

func BenchmarkSubscriptionPublish(b *testing.B) {
	ctx := context.Background()
	pool := qpool.NewQ[any](ctx, 1, 2, nil)
	tree := dmt.NewTree(b.TempDir())
	subscription := NewSubscription(ctx, pool, tree)

	viper.Set("market.subscribe_batch", 100)
	viper.Set("market.subscribe_pace", 0)

	symbols := []string{"BTC/USD", "ETH/USD", "SOL/USD"}

	b.ReportAllocs()

	for b.Loop() {
		if err := subscription.Publish("ticker", symbols); err != nil {
			b.Fatal(err)
		}
	}
}
