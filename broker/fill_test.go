package broker

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/logic"
	. "github.com/theapemachine/symm/signal"
)

func TestSubmitActionResolvesExitQuantity(testingTB *testing.T) {
	Convey("Given a settle action with fraction sizing", testingTB, func() {
		action := &logic.Action{
			Type:     logic.ActionSettlePosition,
			Side:     logic.SideSell,
			Symbol:   "SOL/EUR",
			Fraction: 1,
		}
		holdings := &logic.Balances{
			Inventory: map[string]float64{"SOL": 2.5},
		}

		Convey("When quantity is resolved", func() {
			quantity := resolveActionQuantity(action, holdings)

			So(quantity, ShouldEqual, 2.5)
		})
	})
}

func TestDeskSubmitActionDispatchesToPrivatePool(testingTB *testing.T) {
	Convey("Given a desk and kraken:private subscriber", testingTB, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 2, nil)
		received := make(chan *datura.Artifact, 1)

		pool.Subscribe("kraken:private", func(artifact *datura.Artifact) error {
			received <- artifact

			return nil
		})

		desk := NewDesk(ctx, pool, nil)

		defer desk.Close()

		action := &logic.Action{
			Type:     logic.ActionMarket,
			Side:     logic.SideBuy,
			Symbol:   "BTC/EUR",
			Quantity: 1,
		}

		Convey("When SubmitAction is called", func() {
			err := desk.SubmitAction(action, nil)

			Convey("It should publish an add_order artifact to kraken:private", func() {
				So(err, ShouldBeNil)

				var artifact *datura.Artifact

				select {
				case artifact = <-received:
				case <-time.After(2 * time.Second):
					So("kraken:private order", ShouldEqual, "received")
				}

				So(datura.Peek[string](artifact, "role"), ShouldEqual, "orders")

				destination, destinationErr := artifact.Destination()

				So(destinationErr, ShouldBeNil)
				So(destination, ShouldEqual, "kraken:private")
				So(addOrderTypeFromArtifact(artifact), ShouldEqual, string(trading.Market))
			})
		})
	})
}

func TestDeskSubmitActionSkipsOnStaleTreeQuote(testingTB *testing.T) {
	Convey("Given paper trading with stale tree quotes", testingTB, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 2, nil)
		received := make(chan *datura.Artifact, 1)

		pool.Subscribe("kraken:private", func(artifact *datura.Artifact) error {
			received <- artifact

			return nil
		})

		tree := NewTestTree()
		desk := NewDesk(ctx, pool, tree)

		defer desk.Close()

		viper.Set("trading.model", "paper")
		viper.Set("trading.max_quote_age", time.Second)
		viper.Set("trading.max_spread_bps", 0.0)

		insertIngestAt(tree, "ticker", "BTC/EUR", []byte(
			`{"channel":"ticker","type":"update","data":[{"symbol":"BTC/EUR","last":100,"bid":99,"ask":101}]}`,
		), time.Now().UTC().Add(-2*time.Second))

		action := &logic.Action{
			Type:     logic.ActionMarket,
			Side:     logic.SideBuy,
			Symbol:   "BTC/EUR",
			Quantity: 1,
		}

		Convey("SubmitAction should skip dispatch", func() {
			err := desk.SubmitAction(action, nil)

			So(err, ShouldBeNil)

			select {
			case artifact := <-received:
				So("kraken:private order", ShouldEqual, artifact)
			case <-time.After(200 * time.Millisecond):
			}
		})
	})
}

func TestDeskSubmitActionDispatchesWithFreshTreeQuote(testingTB *testing.T) {
	Convey("Given paper trading with fresh tree quotes", testingTB, func() {
		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 1, 2, nil)
		received := make(chan *datura.Artifact, 1)

		pool.Subscribe("kraken:private", func(artifact *datura.Artifact) error {
			received <- artifact

			return nil
		})

		tree := NewTestTree()
		desk := NewDesk(ctx, pool, tree)

		defer desk.Close()

		viper.Set("trading.model", "paper")
		viper.Set("trading.max_quote_age", time.Minute)
		viper.Set("trading.max_spread_bps", 0.0)

		insertIngest(tree, "ticker", "BTC/EUR", []byte(
			`{"channel":"ticker","type":"update","data":[{"symbol":"BTC/EUR","last":100,"bid":99,"ask":101}]}`,
		))

		action := &logic.Action{
			Type:     logic.ActionMarket,
			Side:     logic.SideBuy,
			Symbol:   "BTC/EUR",
			Quantity: 1,
		}

		Convey("SubmitAction should publish to kraken:private", func() {
			err := desk.SubmitAction(action, nil)

			So(err, ShouldBeNil)

			var artifact *datura.Artifact

			select {
			case artifact = <-received:
			case <-time.After(2 * time.Second):
				So("kraken:private order", ShouldEqual, "received")
			}

			So(datura.Peek[string](artifact, "role"), ShouldEqual, "orders")
			So(addOrderTypeFromArtifact(artifact), ShouldEqual, string(trading.Market))
		})
	})
}

func addOrderTypeFromArtifact(artifact *datura.Artifact) string {
	if artifact == nil {
		return ""
	}

	payload, err := artifact.DecryptPayload()

	if err != nil || len(payload) == 0 {
		return ""
	}

	var envelope struct {
		Params json.RawMessage `json:"params"`
	}

	if unmarshalErr := sonic.Unmarshal(payload, &envelope); unmarshalErr != nil {
		return ""
	}

	var params struct {
		OrderType string `json:"order_type"`
	}

	if unmarshalErr := sonic.Unmarshal(envelope.Params, &params); unmarshalErr != nil {
		return ""
	}

	return params.OrderType
}
