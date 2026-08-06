package tests

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/api-go/v2/pkg/callback"
	sdkkraken "github.com/theapemachine/api-go/v2/pkg/kraken"
	"github.com/theapemachine/api-go/v2/pkg/spot"
	"github.com/theapemachine/symm/cmd"
	"github.com/theapemachine/symm/kraken"
	testtypes "github.com/theapemachine/symm/tests/types"
)

func TestMarketNewMarket(t *testing.T) {
	Convey("Given a list of symbols", t, func() {
		symbols := []*testtypes.Symbol{
			testtypes.NewSymbol("SIM1/USD", 100.0, 42),
		}

		Convey("When NewMarket is initialized", func() {
			market := NewMarket(t.Context(), symbols)
			defer market.Close()

			So(market != nil, ShouldBeTrue)
			So(market.Public != nil, ShouldBeTrue)
			So(market.Private != nil, ShouldBeTrue)
			So(market.Level3 != nil, ShouldBeTrue)
			So(market.State, ShouldEqual, testtypes.Baseline)
		})
	})
}

func TestMarketTransition(t *testing.T) {
	Convey("Given a market in Baseline state", t, func() {
		symbols := []*testtypes.Symbol{
			testtypes.NewSymbol("SIM1/USD", 100.0, 42),
			testtypes.NewSymbol("SIM2/USD", 100.0, 1337),
		}
		market := NewMarket(t.Context(), symbols)
		defer market.Close()

		Convey("When transitioning one symbol to FastPump", func() {
			market.Tick()
			peerBefore := market.latest["SIM2/USD"].Timestamp
			err := market.Transition("SIM1/USD", testtypes.FastPump)

			So(err, ShouldBeNil)
			So(market.State, ShouldEqual, testtypes.FastPump)
			So(market.generators["SIM1/USD"].IgnitionArmed(), ShouldBeTrue)
			So(market.generators["SIM2/USD"].IgnitionArmed(), ShouldBeFalse)
			So(market.latest["SIM2/USD"].Timestamp.After(peerBefore), ShouldBeTrue)

			pumped := market.generators["SIM1/USD"].Step()
			baseline := market.generators["SIM2/USD"].Step()

			So(pumped.ChangePct, ShouldBeGreaterThan, baseline.ChangePct)
		})

		Convey("When transitioning an unknown symbol", func() {
			err := market.Transition("UNKNOWN/USD", testtypes.FastPump)

			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldEqual,
				`market: cannot transition unknown symbol "UNKNOWN/USD"`)
			So(market.State, ShouldEqual, testtypes.Baseline)
		})
	})
}

func TestMarketPace(t *testing.T) {
	Convey("Given an undriven market fixture", t, func() {
		market := NewMarket(t.Context(), []*testtypes.Symbol{
			testtypes.NewSymbol("SIM1/USD", 100.0, 42),
		})
		defer market.Close()

		Convey("Pacing should not delay direct fixture use", func() {
			future := time.Now().Add(time.Hour)
			market.pace(future)

			So(market.clockSet, ShouldBeFalse)
		})
	})
}

func TestMarketWithMarket(t *testing.T) {
	Convey("Given WithMarket test wrapper", t, func() {
		symbols := []*testtypes.Symbol{
			testtypes.NewSymbol("SIM1/USD", 100.0, 42),
		}

		WithMarket(t, symbols, func(market *Market) {
			So(market, ShouldNotBeNil)
			market.Tick()
		})()
	})
}

func TestMarketWithAutoFill(t *testing.T) {
	symbols := []*testtypes.Symbol{
		testtypes.NewSymbol("SIM1/USD", 105.0, 42),
	}

	Convey(
		"Given an executable position lifecycle at the simulated venue",
		t, WithOrders(t, symbols, cmd.Boot, func(market *Market, _ *cmd.System) {
			market.WithAutoFill()
			market.Tick()
			_, private := market.Feeds()
			executions := make(chan *kraken.Execution, 1)
			handler := market.Private.Client().OnReceived.Recurring(
				func(event *callback.Event[*sdkkraken.WebSocketMessage]) {
					execution := kraken.NewExecution(event.Data.Bytes())

					if execution.Channel == "executions" {
						executions <- execution
					}
				},
			)
			defer market.Private.Client().OnReceived.Deregister(handler)

			result, err := private.AddOrder(&spot.AddOrderRequest{
				ClOrdId:   "entry-1",
				OrderType: "market",
				Type:      "buy",
				Volume:    "0.25",
				Pair:      symbols[0].Pair,
			})

			So(err, ShouldBeNil)
			So(result.ID, ShouldHaveLength, 1)

			market.Tick()
			var fill *kraken.Execution

			select {
			case fill = <-executions:
			default:
			}

			So(fill, ShouldNotBeNil)
			So(fill.Data, ShouldHaveLength, 1)
			So(fill.Data[0].OrderID, ShouldEqual, result.ID[0])
			So(fill.Data[0].ClientOrderID, ShouldEqual, "entry-1")
			So(fill.Data[0].Symbol, ShouldEqual, symbols[0].Pair)
			So(fill.Data[0].Side, ShouldEqual, "buy")
			So(fill.Data[0].AvgPrice.Float64(),
				ShouldEqual, market.latest[symbols[0].Pair].Ask)
			So(fill.Data[0].FeeUsdEquiv, ShouldNotBeNil)
			expectedFee := fill.Data[0].Cost.Float64() *
				simulatedTakerFeePercent / percentDenominator
			So(fill.Data[0].FeeUsdEquiv.Float64(),
				ShouldAlmostEqual, expectedFee, 1e-12)
		}))
}

func BenchmarkMarketTick(b *testing.B) {
	symbols := []*testtypes.Symbol{
		testtypes.NewSymbol("SIM1/USD", 100.0, 42),
		testtypes.NewSymbol("SIM2/USD", 200.0, 43),
	}
	market := NewMarket(context.Background(), symbols)
	defer market.Close()

	for b.Loop() {
		market.Tick()
	}
}
