package response

import (
	"context"
	"fmt"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
)

func TestNewFillSimulator(testingTB *testing.T) {
	Convey("Given a tree and context", testingTB, func() {
		ctx := context.Background()
		tree := dmt.NewTree("")

		Convey("When NewFillSimulator constructs the simulator", func() {
			fills := NewFillSimulator(ctx, tree)

			Convey("Then it should keep the runtime dependencies", func() {
				So(fills.ctx, ShouldNotBeNil)
				So(fills.cancel, ShouldNotBeNil)
				So(fills.tree, ShouldEqual, tree)
				So(fills.latency, ShouldNotBeNil)
			})
		})
	})
}

func TestPreflight(testingTB *testing.T) {
	Convey("Given a fill simulator", testingTB, func() {
		setFillConfig()

		ctx := context.Background()
		tree := dmt.NewTree("")
		fills := NewFillSimulator(ctx, tree)

		Convey("When the order is nil", func() {
			err := fills.Preflight(nil)

			Convey("Then preflight should be a no-op", func() {
				So(err, ShouldBeNil)
			})
		})

		Convey("When the symbol has no quote", func() {
			err := fills.Preflight(fillOrder("BTC/USD", "buy", "market", 0.1))

			Convey("Then preflight should reject the order", func() {
				So(err, ShouldNotBeNil)
			})
		})

		Convey("When quote and fee data exist", func() {
			insertTicker(tree, "BTC/USD")
			insertAssetPairs(tree, "BTC/USD", 0.4, 0.25)

			err := fills.Preflight(fillOrder("BTC/USD", "buy", "market", 0.1))

			Convey("Then preflight should accept the order", func() {
				So(err, ShouldBeNil)
			})
		})
	})
}

func TestSimulate(testingTB *testing.T) {
	Convey("Given a fill simulator", testingTB, func() {
		setFillConfig()

		ctx := context.Background()
		tree := dmt.NewTree("")
		fills := NewFillSimulator(ctx, tree)

		Convey("When the order is nil", func() {
			fill, err := fills.Simulate(nil, "order-1")

			Convey("Then simulation should reject it", func() {
				So(fill, ShouldBeNil)
				So(err, ShouldNotBeNil)
			})
		})

		Convey("When the symbol has no quote", func() {
			fill, err := fills.Simulate(
				fillOrder("BTC/USD", "buy", "market", 0.1),
				"order-1",
			)

			Convey("Then simulation should fail", func() {
				So(fill, ShouldBeNil)
				So(err, ShouldNotBeNil)
			})
		})

		Convey("When ticker and fee data exist", func() {
			insertTicker(tree, "BTC/USD")
			insertAssetPairs(tree, "BTC/USD", 0.4, 0.25)

			fill, err := fills.Simulate(
				fillOrder("BTC/USD", "buy", "market", 0.1),
				"order-1",
			)

			Convey("Then simulation should produce a priced execution", func() {
				So(err, ShouldBeNil)
				So(fill, ShouldNotBeNil)
				So(datura.Peek[string](fill, "symbol"), ShouldEqual, "BTC/USD")
				So(datura.Peek[string](fill, "fee_ccy"), ShouldEqual, "USD")
				So(datura.Peek[float64](fill, "last_price"), ShouldBeGreaterThan, 0)
				So(math.Abs(datura.Peek[float64](fill, "fee")-0.0402), ShouldBeLessThan, 1e-9)
			})
		})

		Convey("When the quote is stored in the live ingest layout", func() {
			insertLiveFrame(tree, "ticker", []byte(
				`{"channel":"ticker","type":"update","data":[{"symbol":"ETH/USD","last":200,"bid":199,"ask":201}]}`,
			))
			insertAssetPairs(tree, "ETH/USD", 0.4, 0.25)

			fill, err := fills.Simulate(
				fillOrder("ETH/USD", "sell", "market", 0.5),
				"order-2",
			)

			Convey("Then simulation should consume the live-shaped frame", func() {
				So(err, ShouldBeNil)
				So(fill, ShouldNotBeNil)
				So(datura.Peek[string](fill, "symbol"), ShouldEqual, "ETH/USD")
				So(datura.Peek[float64](fill, "order_qty"), ShouldEqual, 0.5)
			})
		})
	})
}

func TestError(testingTB *testing.T) {
	Convey("Given an unreadable latency profile", testingTB, func() {
		viper.Set("trading.paper.latency_profile", "missing-latency-profile.yml")

		Convey("When NewFillSimulator loads latency", func() {
			fills := NewFillSimulator(context.Background(), dmt.NewTree(""))

			Convey("Then Error should expose the load failure", func() {
				So(fills.Error(), ShouldNotBeNil)
			})
		})
	})
}

func TestClose(testingTB *testing.T) {
	Convey("Given a fill simulator", testingTB, func() {
		fills := NewFillSimulator(context.Background(), dmt.NewTree(""))

		Convey("When Close is called", func() {
			err := fills.Close()

			Convey("Then it should cancel its context", func() {
				So(err, ShouldBeNil)

				select {
				case <-fills.ctx.Done():
					So(true, ShouldBeTrue)
				default:
					So("context", ShouldEqual, "cancelled")
				}
			})
		})
	})
}

func setFillConfig() {
	viper.Set("trading.paper.latency_profile", "")
	viper.Set("trading.paper.slippage_bps", 0.0)
	viper.Set("trading.max_quote_age", 0)
	viper.Set("trading.max_spread_bps", 0.0)
	viper.Set("trading.max_slippage_bps", 0.0)
	viper.Set("trading.replay.min_depth_coverage", 0.0)
}

func fillOrder(symbol, side, orderType string, qty float64) *datura.Artifact {
	order := datura.Acquire("paper", datura.Artifact_Type_json)
	order.Poke(symbol, "symbol")
	order.Poke(side, "side")
	order.Poke(qty, "order_qty")
	order.Poke(orderType, "order_type")

	return order
}

func insertTicker(tree *dmt.Tree, symbol string) {
	insertIngest(tree, "ticker", symbol, []byte(fmt.Sprintf(
		`{"channel":"ticker","type":"update","data":[{"symbol":%q,"last":100,"bid":99.5,"ask":100.5}]}`,
		symbol,
	)))
}

func insertLiveFrame(tree *dmt.Tree, role string, frame []byte) {
	artifact := datura.Acquire("websocket", datura.Artifact_Type_json).
		WithRole(role).
		WithPayload(frame)

	if symbol := datura.Peek[string](artifact, "data", 0, "symbol"); symbol != "" {
		artifact.WithScope(symbol)
	}

	tree.InsertArtifact(artifact.Prefix("role", "timestamp"), artifact)
}

func insertAssetPairs(tree *dmt.Tree, symbol string, taker, maker float64) {
	payload := []byte(fmt.Sprintf(
		`{"wsname":%q,"fees":[[0,%g]],"fees_maker":[[0,%g]],"fee_volume_currency":"ZUSD"}`,
		symbol, taker, maker,
	))

	insertIngest(tree, "assetpairs", symbol, payload)
}

func insertIngest(tree *dmt.Tree, role, scope string, payload []byte) {
	artifact := datura.Acquire("test", datura.Artifact_Type_json).
		WithRole(role).
		WithScope(scope).
		WithPayload(payload)

	if wire := artifact.Pack(); len(wire) > 0 {
		tree.Insert(artifact.Prefix(), wire)
	}
}
