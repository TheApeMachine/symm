package public

import (
	"context"
	"fmt"
	"testing"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/qpool"
)

func insertInstrumentCatalog(tree *dmt.Tree, pairs []map[string]string) {
	payload, err := sonic.Marshal(map[string]any{
		"channel": "instrument",
		"type":    "snapshot",
		"data": map[string]any{
			"pairs": pairs,
		},
	})

	if err != nil {
		panic(err)
	}

	artifact := datura.Acquire("kraken:public", datura.APPJSON).
		WithPayload(payload)
	artifact.WithRole("instrument")
	artifact.WithScope("snapshot")

	tree.Insert(artifact.Prefix(), artifact.Pack())
}

func TestSubscriptionEnsureWaitsForCatalog(testingTB *testing.T) {
	Convey("Given no instrument catalog in the tree", testingTB, func() {
		ctx, cancel := context.WithCancel(context.Background())

		defer cancel()

		viper.Set("market.quote_currency", "USD")

		pool := qpool.NewQ[any](ctx, 1, 2, nil)
		tree := dmt.NewTree(testingTB.TempDir())
		subscription := NewSubscription(ctx, pool, tree)

		received := make(chan *datura.Artifact, 4)
		pool.Subscribe("kraken:public", func(artifact *datura.Artifact) error {
			received <- artifact
			return nil
		})

		err := subscription.Ensure()

		Convey("It should request the catalog and remain unarmed", func() {
			So(err, ShouldBeNil)
			So(subscription.Armed(), ShouldBeFalse)
			So(subscription.Symbols(), ShouldBeEmpty)

			artifact := <-received

			So(datura.Peek[string](artifact, "params", "channel"), ShouldEqual, "instrument")
		})
	})
}

func TestSubscriptionEnsureArmsAllUSDPairs(testingTB *testing.T) {
	Convey("Given the instrument catalog in the tree", testingTB, func() {
		ctx, cancel := context.WithCancel(context.Background())

		defer cancel()

		viper.Set("market.quote_currency", "USD")
		viper.Set("market.subscribe_batch", 100)
		viper.Set("market.subscribe_pace", 0)

		pool := qpool.NewQ[any](ctx, 1, 2, nil)
		tree := dmt.NewTree(testingTB.TempDir())

		insertInstrumentCatalog(tree, []map[string]string{
			{"symbol": "BTC/USD", "quote": "USD"},
			{"symbol": "ETH/USD", "quote": "USD"},
		})

		subscription := NewSubscription(ctx, pool, tree)

		received := make(chan *datura.Artifact, 8)
		pool.Subscribe("kraken:public", func(artifact *datura.Artifact) error {
			received <- artifact
			return nil
		})

		err := subscription.Ensure()

		Convey("It should subscribe ticker, book, and trade for every USD pair", func() {
			So(err, ShouldBeNil)
			So(subscription.Armed(), ShouldBeTrue)

			channels := make([]string, 0, 4)

			for range 4 {
				artifact := <-received

				channel := datura.Peek[string](artifact, "params", "channel")
				channels = append(channels, channel)

				if channel == "instrument" {
					continue
				}

				payload := string(artifact.DecryptPayload())

				So(payload, ShouldContainSubstring, `"BTC/USD"`)
				So(payload, ShouldContainSubstring, `"ETH/USD"`)
			}

			So(channels, ShouldContain, "instrument")
			So(channels, ShouldContain, "ticker")
			So(channels, ShouldContain, "book")
			So(channels, ShouldContain, "trade")
		})
	})
}

func TestSubscriptionSymbols(testingTB *testing.T) {
	Convey("Given an instrument catalog in the tree", testingTB, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 2, nil)
		tree := dmt.NewTree(testingTB.TempDir())

		viper.Set("market.quote_currency", "USD")

		subscription := NewSubscription(ctx, pool, tree)

		insertInstrumentCatalog(tree, []map[string]string{
			{"symbol": "BTC/USD", "quote": "USD"},
			{"symbol": "ETH/USD", "quote": "USD"},
			{"symbol": "ETH/EUR", "quote": "EUR"},
		})

		Convey("It should return every USD pair and exclude other quotes", func() {
			So(subscription.Symbols(), ShouldResemble, []string{"BTC/USD", "ETH/USD"})
		})
	})
}

func TestSubscriptionSymbolsAllUSDPairs(testingTB *testing.T) {
	Convey("Given many USD pairs in the catalog", testingTB, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 2, nil)
		tree := dmt.NewTree(testingTB.TempDir())

		viper.Set("market.quote_currency", "USD")

		subscription := NewSubscription(ctx, pool, tree)

		catalog := make([]map[string]string, 0, 120)

		for index := range 120 {
			catalog = append(catalog, map[string]string{
				"symbol": fmt.Sprintf("SYM%d/USD", index),
				"quote":  "USD",
			})
		}

		catalog = append(catalog, map[string]string{
			"symbol": "BTC/EUR",
			"quote":  "EUR",
		})

		insertInstrumentCatalog(tree, catalog)

		Convey("It should return every USD pair with no cap", func() {
			So(len(subscription.Symbols()), ShouldEqual, 120)
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
