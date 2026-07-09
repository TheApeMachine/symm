package trader

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/strategy"

	. "github.com/smartystreets/goconvey/convey"
)

type plannerSocket struct {
	channels map[string]chan []byte
}

func (socket *plannerSocket) Observe(channel string) chan []byte {
	if socket.channels == nil {
		socket.channels = map[string]chan []byte{}
	}

	socket.channels[channel] = make(chan []byte, 8)
	return socket.channels[channel]
}

func (socket *plannerSocket) Ticker(_ []string) (kraken.TickerDataSlice, error) {
	return nil, nil
}

type plannerPrivate struct {
	channels map[string]chan []byte
	orders   []*kraken.Order
	schedule websocket.FeeSchedule
}

func (private *plannerPrivate) Observe(channel string) chan []byte {
	if private.channels == nil {
		private.channels = map[string]chan []byte{}
	}

	private.channels[channel] = make(chan []byte, 8)
	return private.channels[channel]
}

func (private *plannerPrivate) Submit(order *kraken.Order) error {
	private.orders = append(private.orders, order)
	return nil
}

func (private *plannerPrivate) TradeVolume(_ []string) (websocket.FeeSchedule, error) {
	return private.schedule, nil
}

func (private *plannerPrivate) Close() {
}

func TestPlannerEntry(testingTB *testing.T) {
	Convey("Given Planner with broker-priced round-trip friction", testingTB, func() {
		previousQuote := viper.GetString("market.quote_currency")
		viper.Set("market.quote_currency", "USD")
		defer viper.Set("market.quote_currency", previousQuote)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		public := &plannerSocket{}
		private := &plannerPrivate{
			schedule: websocket.FeeSchedule{
				Pairs: map[string]websocket.FeeRates{
					"MANA/USD": {Taker: 0.001},
				},
			},
		}
		price := broker.NewPrice(ctx, public, private)
		defer price.Close()

		public.channels["instrument"] <- []byte(`{
			"channel": "instrument",
			"data": {
				"pairs": [{
					"symbol": "MANA/USD",
					"quote": "USD",
					"status": "online"
				}]
			}
		}`)
		waitForPlanner(testingTB, func() bool {
			select {
			case public.channels["ticker"] <- []byte(`[{
				"symbol": "MANA/USD",
				"bid": 100,
				"ask": 100.1,
				"last": 100.05
			}]`):
			default:
			}

			friction, ok := price.RoundTripFriction("MANA/USD")
			return ok && friction.Sign() > 0
		})

		planner := NewPlanner(nil, price)

		Convey("When the forecast clears friction and the causal baseline", func() {
			intent, ok := planner.Entry(plannerThesis(math.Log1p(0.004), 0.8, 0.4))

			Convey("Then entry is accepted in positive utility space", func() {
				So(ok, ShouldBeTrue)
				So(intent.Action, ShouldEqual, strategy.ActionBuy)
				So(intent.Edge, ShouldBeGreaterThan, 0)
				So(intent.Velocity, ShouldEqual, intent.Edge)
			})
		})

		Convey("When the forecast does not clear friction", func() {
			_, ok := planner.Entry(plannerThesis(math.Log1p(0.001), 0.8, 0.4))

			Convey("Then entry is rejected", func() {
				So(ok, ShouldBeFalse)
			})
		})

		Convey("When confidence does not clear its causal baseline", func() {
			_, ok := planner.Entry(plannerThesis(math.Log1p(0.004), 0.4, 0.4))

			Convey("Then entry is rejected", func() {
				So(ok, ShouldBeFalse)
			})
		})
	})
}

func TestPlannerExit(testingTB *testing.T) {
	Convey("Given Planner comparing hold utility to replacement utility", testingTB, func() {
		planner := &Planner{}
		thesis := strategy.NewThesis()
		replacement := strategy.Intent{
			Symbol:     "SOL/USD",
			Action:     strategy.ActionBuy,
			Velocity:   0.004,
			Confidence: 0.7,
			Thesis:     thesis,
		}
		thesis.AddEvidence("entry", replacement)
		thesis.AddEvidence("holdings", map[string]broker.PositionData{
			"MANA/USD": {
				Symbol:    "MANA/USD",
				ReturnPct: 0.001,
			},
		})

		Convey("When replacement utility is higher", func() {
			intent, ok := planner.Exit(thesis)

			Convey("Then rotation exits the held position", func() {
				So(ok, ShouldBeTrue)
				So(intent.Symbol, ShouldEqual, "MANA/USD")
				So(intent.Action, ShouldEqual, strategy.ActionSell)
				So(intent.Edge, ShouldAlmostEqual, 0.003)
			})
		})

		Convey("When replacement utility is not higher", func() {
			thesis := strategy.NewThesis()
			replacement.Velocity = 0.001
			thesis.AddEvidence("entry", replacement)
			thesis.AddEvidence("holdings", map[string]broker.PositionData{
				"MANA/USD": {
					Symbol:    "MANA/USD",
					ReturnPct: 0.001,
				},
			})

			_, ok := planner.Exit(thesis)

			Convey("Then rotation is rejected", func() {
				So(ok, ShouldBeFalse)
			})
		})

		Convey("When the only holding is the replacement symbol", func() {
			thesis := strategy.NewThesis()
			thesis.AddEvidence("entry", replacement)
			thesis.AddEvidence("holdings", map[string]broker.PositionData{
				"SOL/USD": {
					Symbol:    "SOL/USD",
					ReturnPct: -0.1,
				},
			})

			_, ok := planner.Exit(thesis)

			Convey("Then same-symbol churn is rejected", func() {
				So(ok, ShouldBeFalse)
			})
		})
	})
}

func TestPlannerUpdate(testingTB *testing.T) {
	Convey("Given Planner with broker desk ownership", testingTB, func() {
		previousQuote := viper.GetString("market.quote_currency")
		previousFraction := viper.GetFloat64("trading.sizing.base_fraction")
		previousNormalSlots := viper.GetInt("trading.slots.normal")
		previousReservedSlots := viper.GetInt("trading.slots.reserved")
		viper.Set("market.quote_currency", "USD")
		viper.Set("trading.sizing.base_fraction", 0.05)
		viper.Set("trading.slots.normal", 1)
		viper.Set("trading.slots.reserved", 1)
		defer viper.Set("market.quote_currency", previousQuote)
		defer viper.Set("trading.sizing.base_fraction", previousFraction)
		defer viper.Set("trading.slots.normal", previousNormalSlots)
		defer viper.Set("trading.slots.reserved", previousReservedSlots)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		public := &plannerSocket{}
		private := &plannerPrivate{
			schedule: websocket.FeeSchedule{
				Pairs: map[string]websocket.FeeRates{
					"MANA/USD": {Taker: 0.001},
					"SOL/USD":  {Taker: 0.001},
				},
			},
		}
		desk, err := broker.NewDesk(ctx, public, private, make(chan []byte, 8))
		So(err, ShouldBeNil)
		price := broker.NewPrice(ctx, public, private)
		defer price.Close()
		planner := NewPlanner(desk, price)

		done := make(chan error, 1)
		go func() {
			done <- desk.Run()
		}()
		defer func() {
			cancel()
			<-done
		}()

		private.channels["balances"] <- []byte(`[{
			"asset": "USD",
			"asset_class": "currency",
			"balance": 200,
			"available": 200
		}]`)
		public.channels["instrument"] <- []byte(`{
			"channel": "instrument",
			"data": {
				"pairs": [{
					"symbol": "MANA/USD",
					"quote": "USD",
					"status": "online"
				}, {
					"symbol": "SOL/USD",
					"quote": "USD",
					"status": "online"
				}]
			}
		}`)
		waitForPlanner(testingTB, func() bool {
			select {
			case public.channels["ticker"] <- []byte(`[{
				"symbol": "SOL/USD",
				"bid": 100,
				"ask": 100.1,
				"last": 100.05
			}]`):
			default:
			}

			_, ok := price.Entry("SOL/USD")
			return ok && desk.Ready()
		})

		Convey("When a thesis clears entry utility", func() {
			intents, err := planner.Update(plannerSymbolThesis(
				"SOL/USD",
				math.Log1p(0.01),
				0.8,
				0.4,
			))

			Convey("Then the planner submits the buy through the broker desk", func() {
				So(err, ShouldBeNil)
				So(intents, ShouldHaveLength, 1)
				So(intents[0].Action, ShouldEqual, strategy.ActionBuy)
				So(private.orders, ShouldHaveLength, 1)
				params := private.orders[0].Params.(kraken.LimitOrderParams)
				So(params.Side, ShouldEqual, "buy")
				So(params.Symbol, ShouldEqual, "SOL/USD")
			})
		})

		Convey("When a replacement thesis beats an existing holding", func() {
			private.orders = private.orders[:0]
			err := desk.Buy("MANA/USD", 0.05, *decimalFromFloat(100), false)
			So(err, ShouldBeNil)
			private.orders = private.orders[:0]

			intents, err := planner.Update(plannerSymbolThesis(
				"SOL/USD",
				math.Log1p(0.01),
				0.8,
				0.4,
			))

			Convey("Then the planner submits the rotation through the broker desk", func() {
				So(err, ShouldBeNil)
				So(intents, ShouldHaveLength, 2)
				So(intents[0].Action, ShouldEqual, strategy.ActionSell)
				So(intents[1].Action, ShouldEqual, strategy.ActionBuy)
				So(private.orders, ShouldHaveLength, 2)
				exitParams := private.orders[0].Params.(kraken.LimitOrderParams)
				entryParams := private.orders[1].Params.(kraken.LimitOrderParams)
				So(exitParams.Side, ShouldEqual, "sell")
				So(exitParams.Symbol, ShouldEqual, "MANA/USD")
				So(entryParams.Side, ShouldEqual, "buy")
				So(entryParams.Symbol, ShouldEqual, "SOL/USD")
			})
		})
	})
}

func plannerThesis(
	forecast float64,
	confidence float64,
	entryBaseline float64,
) *strategy.Thesis {
	return plannerSymbolThesis("MANA/USD", forecast, confidence, entryBaseline)
}

func plannerSymbolThesis(
	symbol string,
	forecast float64,
	confidence float64,
	entryBaseline float64,
) *strategy.Thesis {
	thesis := strategy.NewThesis()
	thesis.AddEvidence("symbol", symbol)
	thesis.AddEvidence("resonance", logic.ResonanceOutcome{
		ReturnForecast: forecast,
	})
	thesis.AddEvidence("causal", algorithm.PearlOutput{
		Confidence:    confidence,
		EntryBaseline: entryBaseline,
	})

	return thesis
}

func decimalFromFloat(value float64) *decimal.Decimal {
	return decimal.NewFromFloat64(value)
}

func waitForPlanner(testingTB *testing.T, ready func() bool) {
	deadline := time.After(time.Second)

	for {
		if ready() {
			return
		}

		select {
		case <-deadline:
			testingTB.Fatal("planner price state did not become ready")
		default:
			time.Sleep(time.Millisecond)
		}
	}
}
