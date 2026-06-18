package trader

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/kraken/paper"
	"github.com/theapemachine/symm/kraken/user"
	"github.com/theapemachine/symm/logic"
	. "github.com/theapemachine/symm/signal"
)

func TestCryptoMeasureCycleSealsCognitiveMemory(testingTB *testing.T) {
	Convey("Given tree classifier measurements for a scope", testingTB, func() {
		pool := productionPool(testingTB)

		defer pool.Close()

		tree := NewTestTree()
		crypto := NewCrypto(context.Background(), pool, tree)

		defer crypto.Close()

		viper.Set("cognitive.beam_width", 4)
		viper.Set("cognitive.beam_hops", 3)
		viper.Set("cognitive.rem_interval", time.Hour)
		defer viper.Reset()

		insertClassifierMeasurement(tree, "fluid", "BTC/USD", 1, 0.82)
		insertClassifierMeasurement(tree, "hawkes", "BTC/USD", 2, 0.76)

		crypto.measure()

		Convey("It should observe, seal, and expose a cognitive reading", func() {
			reading, ok := crypto.memory.ReadingForScope("BTC/USD")

			So(ok, ShouldBeTrue)
			So(reading, ShouldNotBeNil)
			So(reading.Scope, ShouldEqual, "BTC/USD")
			So(len(reading.Sequence), ShouldBeGreaterThan, 0)
			So(string(reading.Sequence), ShouldContainSubstring, "BTC/USD")
		})
	})
}

func TestCryptoSettleStoryForwardFeedback(testingTB *testing.T) {
	Convey("Given a story measurement and ticker mark in the tree", testingTB, func() {
		pool := productionPool(testingTB)

		defer pool.Close()

		tree := NewTestTree()
		crypto := NewCrypto(context.Background(), pool, tree)

		defer crypto.Close()

		viper.Set("market.story.measurement_max_age", 30*time.Second)
		viper.Set("market.story.forward_return_min_samples", 1)
		viper.Set("market.story.forward_return_significance_z", 0.5)
		defer viper.Reset()

		anchorAt := time.Now().Add(-31 * time.Second)
		measurement := logic.Measurement{
			Source:     logic.SourceFluid,
			Symbol:     "ETH/USD",
			Price:      100,
			Strength:   0.5,
			Confidence: 0.8,
			Position:   logic.PositionTypeLong,
			Category:   logic.CategoryLaminar,
			ObservedAt: anchorAt,
		}
		payload, _ := json.Marshal(measurement)

		So(crypto.story.Update(datura.Acquire("test", datura.Artifact_Type_json).
			WithRole("measurement").
			WithScope("ETH/USD").
			WithPayload(payload)), ShouldBeNil)

		insertTickerQuote(tree, "ETH/USD", 101, 100.5, 101.5)

		crypto.measure()

		Convey("It should settle forward feedback through the desk quote cache", func() {
			feedback := crypto.story.FeedbackFor("ETH/USD", logic.SourceFluid)

			So(feedback, ShouldNotBeNil)
			So(feedback.Samples, ShouldEqual, 1)

			calibrated := measurementFromStory(crypto, "ETH/USD", logic.SourceFluid)

			So(calibrated.ExpectedMoveBps, ShouldAlmostEqual, 160, 0.01)
			So(calibrated.Strength, ShouldAlmostEqual, 1.25, 0.01)
		})
	})
}

func measurementFromStory(
	crypto *Crypto,
	symbol string,
	source logic.SourceType,
) logic.Measurement {
	for _, measurement := range crypto.story.Measurements() {
		if measurement.Symbol == symbol && measurement.Source == source {
			return measurement
		}
	}

	return logic.Measurement{}
}

func TestCryptoApplyPlaybookActionsPrefersExits(testingTB *testing.T) {
	Convey("Given tree classifier measurements and wallet holdings", testingTB, func() {
		pool := productionPool(testingTB)

		defer pool.Close()

		received := make(chan *datura.Artifact, 4)

		pool.Subscribe("kraken:private", func(artifact *datura.Artifact) error {
			received <- artifact

			return nil
		})

		tree := NewTestTree()
		crypto := NewCrypto(context.Background(), pool, tree)

		defer crypto.Close()

		insertClassifierMeasurement(tree, "exhaust", "SOL/EUR", 1, 0.85)
		insertClassifierMeasurement(tree, "pumpdump", "BTC/EUR", 3, 0.82)
		insertClassifierMeasurement(tree, "sentiment", "BTC/EUR", 1, 0.82)
		insertTickerQuote(tree, "SOL/EUR", 100, 99.5, 100.5)
		insertTickerQuote(tree, "BTC/EUR", 50000, 49950, 50050)

		viper.Set("trading.model", "paper")
		viper.Set("trading.max_quote_age", time.Minute)
		viper.Set("trading.max_spread_bps", 0)
		viper.Set("trading.paper.slippage_bps", 5)

		crypto.wallet = &user.Balances{
			Inventory: map[string]float64{"SOL/EUR": 1.5},
		}

		crypto.measure()

		crypto.syncStoryBalances(crypto.storyHoldings())
		actions := sortActionsExitsFirst(crypto.story.Actions())

		Convey("It should evaluate exit and entry actions from tree measurements", func() {
			So(len(actions), ShouldBeGreaterThanOrEqualTo, 2)

			hasExit := false
			hasEntry := false

			for _, action := range actions {
				if action == nil {
					continue
				}

				if action.Type.IsExit() {
					hasExit = true
					So(action.Symbol, ShouldEqual, "SOL/EUR")
				}

				if action.Type == logic.ActionMarket {
					hasEntry = true
					So(action.Symbol, ShouldEqual, "BTC/EUR")
				}
			}

			So(hasExit, ShouldBeTrue)
			So(hasEntry, ShouldBeTrue)
			So(actions[0].Type.IsExit(), ShouldBeTrue)
			So(actions[1].Type, ShouldEqual, logic.ActionMarket)
		})

		Convey("When applyPlaybookActions runs after measure", func() {
			drainPrivateOrders(received)
			crypto.applyPlaybookActions()

			captured := collectPrivateOrders(received, 2)

			Convey("It should dispatch kraken:private order artifacts with exits first", func() {
				So(len(captured), ShouldBeGreaterThanOrEqualTo, 1)

				for _, artifact := range captured {
					So(datura.Peek[string](artifact, "role"), ShouldEqual, "orders")

					destination, destinationErr := artifact.Destination()

					So(destinationErr, ShouldBeNil)
					So(destination, ShouldEqual, "kraken:private")
				}

				So(isExitOrderArtifact(captured[0]), ShouldBeTrue)

				if len(captured) >= 2 {
					So(isExitOrderArtifact(captured[0]), ShouldBeTrue)
					So(isExitOrderArtifact(captured[1]), ShouldBeFalse)
				}
			})
		})
	})
}

func TestCryptoApplyPlaybookActionsRespectsSideline(testingTB *testing.T) {
	Convey("Given sidelined and active scopes with exit and entry actions", testingTB, func() {
		pool := productionPool(testingTB)

		defer pool.Close()

		received := make(chan *datura.Artifact, 4)

		pool.Subscribe("kraken:private", func(artifact *datura.Artifact) error {
			received <- artifact

			return nil
		})

		tree := NewTestTree()
		crypto := NewCrypto(context.Background(), pool, tree)

		defer crypto.Close()

		insertClassifierMeasurement(tree, "exhaust", "SOL/EUR", 1, 0.85)
		insertClassifierMeasurement(tree, "pumpdump", "BTC/EUR", 3, 0.82)
		insertClassifierMeasurement(tree, "sentiment", "BTC/EUR", 1, 0.82)
		insertTickerQuote(tree, "SOL/EUR", 100, 99.5, 100.5)
		insertTickerQuote(tree, "BTC/EUR", 50000, 49950, 50050)

		viper.Set("trading.model", "paper")
		viper.Set("trading.max_quote_age", time.Minute)
		viper.Set("trading.max_spread_bps", 0)
		viper.Set("trading.paper.slippage_bps", 5)

		crypto.wallet = &user.Balances{
			Inventory: map[string]float64{"SOL/EUR": 1.5},
		}

		crypto.measure()

		entryReading, entryReadingOK := crypto.memory.ReadingForScope("BTC/EUR")

		So(entryReadingOK, ShouldBeTrue)
		So(entryReading, ShouldNotBeNil)
		entryReading.Sideline = true
		So(crypto.memory.Sideline("BTC/EUR"), ShouldBeTrue)

		crypto.syncStoryBalances(crypto.storyHoldings())
		actions := sortActionsExitsFirst(crypto.story.Actions())

		Convey("It should still evaluate exit and entry actions from tree measurements", func() {
			So(len(actions), ShouldBeGreaterThanOrEqualTo, 2)

			hasExit := false
			hasEntry := false

			for _, action := range actions {
				if action == nil {
					continue
				}

				if action.Type.IsExit() {
					hasExit = true
					So(action.Symbol, ShouldEqual, "SOL/EUR")
				}

				if action.Type == logic.ActionMarket {
					hasEntry = true
					So(action.Symbol, ShouldEqual, "BTC/EUR")
				}
			}

			So(hasExit, ShouldBeTrue)
			So(hasEntry, ShouldBeTrue)
		})

		Convey("When applyPlaybookActions runs after measure", func() {
			drainPrivateOrders(received)
			crypto.applyPlaybookActions()

			captured := collectPrivateOrders(received, 2)

			Convey("It should dispatch exits but block sidelined entries", func() {
				So(len(captured), ShouldEqual, 1)
				So(isExitOrderArtifact(captured[0]), ShouldBeTrue)
			})
		})
	})
}

func TestCryptoBootstrapWalletFromSubscribe(testingTB *testing.T) {
	Convey("Given paper websocket and trader sharing a pool", testingTB, func() {
		viper.Set("market.quote_currency", "USD")
		viper.Set("trading.paper.wallet.usd", 200)

		ctx := context.Background()
		pool := productionPool(testingTB)

		defer pool.Close()

		tree := NewTestTree()
		paperSocket := paper.NewWebSocket(ctx, pool, tree)

		go paperSocket.Run()

		defer paperSocket.Close()

		crypto := NewCrypto(ctx, pool, tree)

		defer crypto.Close()

		crypto.bootstrapWallet()

		Convey("It should hydrate wallet and publishable connect snapshot", func() {
			So(crypto.wallet, ShouldNotBeNil)
			So(walletQuoteBalance(crypto.wallet, "USD"), ShouldEqual, 200)

			frames := crypto.ConnectSnapshotFrames()

			hasWallet := false

			for _, frame := range frames {
				if frame["type"] != "wallet" {
					continue
				}

				hasWallet = true
				So(frame["Balance"], ShouldEqual, 200)
			}

			So(hasWallet, ShouldBeTrue)
		})
	})
}

func walletQuoteBalance(balances *user.Balances, asset string) float64 {
	if balances == nil {
		return 0
	}

	for _, row := range balances.Asset {
		if row.Asset == asset {
			return row.Balance
		}
	}

	return 0
}
