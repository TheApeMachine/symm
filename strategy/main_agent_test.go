package strategy

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/types"
)

func testPrice() *broker.Price {
	instrument := broker.NewInstrumentWithQuote("USD")
	price := broker.NewPrice(nil, instrument)
	feeRate := decimal.NewFromFloat64(0.26)
	price.SetFee("BTC/USD", kraken.TradeVolumeFee{Fee: feeRate})
	price.SetFee("ETH/USD", kraken.TradeVolumeFee{Fee: feeRate})
	price.SetFee("SOL/USD", kraken.TradeVolumeFee{Fee: feeRate})

	return price
}

func TestMainAgent(t *testing.T) {
	Convey("Given a MainAgent operating with an associative cognition engine", t, func() {
		initialCash := decimal.NewFromInt64(1000)
		engine := cognition.NewEngine(cognition.DefaultConfig())
		priceSvc := testPrice()
		mainAgent := NewMainAgent(initialCash, "paper", nil, priceSvc, engine)

		So(mainAgent.ID(), ShouldEqual, 0)
		So(mainAgent.Status(), ShouldEqual, "simulated")
		So(mainAgent.fills, ShouldEqual, 0)
		So(mainAgent.decisions, ShouldEqual, 0)
		So(mainAgent.cash.Cmp(initialCash), ShouldEqual, 0)

		Convey("When an upward precursor signal arrives with high confidence", func() {
			price50k := decimal.NewFromInt64(50000)
			envelope := &types.Envelope{
				TickerData: kraken.TickerData{
					Symbol: "BTC/USD",
					Last:   price50k,
				},
			}
			entryCtx := []byte("entry_precursor_pattern_123")
			decision := ActionDecision{
				Action:     ActionEnter,
				Context:    entryCtx,
				Confidence: 0.85,
				Contrast:   2.4,
				Support:    12,
			}

			mainAgent.Step(envelope, decision)

			So(mainAgent.fills, ShouldEqual, 1)
			So(mainAgent.decisions, ShouldEqual, 1)
			So(len(mainAgent.positions), ShouldEqual, 1)

			holding := mainAgent.positions["BTC/USD"]
			So(holding, ShouldNotBeNil)
			So(holding.EntryPrice.String(), ShouldStartWith, "50000")

			cashDec, _ := decimal.NewFromString(mainAgent.cash.String())
			So(cashDec.Cmp(initialCash), ShouldBeLessThan, 0)

			Convey("When the price rises and position is marked to market", func() {
				price55k := decimal.NewFromInt64(55000)
				envelopeHigher := &types.Envelope{
					TickerData: kraken.TickerData{
						Symbol: "BTC/USD",
						Last:   price55k,
					},
				}

				mainAgent.Step(envelopeHigher, ActionDecision{Action: ActionWait, Confidence: 0.6})

				So(mainAgent.unrealized.Sign(), ShouldBeGreaterThan, 0)
				So(mainAgent.equity.Cmp(initialCash), ShouldBeGreaterThan, 0)

				Convey("When an exit action triggers an exit", func() {
					envelopeExit := &types.Envelope{
						TickerData: kraken.TickerData{
							Symbol: "BTC/USD",
							Last:   price55k,
						},
					}
					downDecision := ActionDecision{
						Action:     ActionExit,
						Confidence: 0.80,
						Contrast:   1.8,
						Support:    15,
					}

					mainAgent.Step(envelopeExit, downDecision)

					So(mainAgent.fills, ShouldEqual, 2)
					So(len(mainAgent.positions), ShouldEqual, 0)
					So(mainAgent.wins, ShouldEqual, 1)
					So(mainAgent.realized.Sign(), ShouldBeGreaterThan, 0)
					So(len(mainAgent.outcomes), ShouldEqual, 1)
					So(mainAgent.outcomes[0].ReturnBp, ShouldBeGreaterThan, 0)

					evaluation := engine.Evaluate(entryCtx)
					So(evaluation.WinnerClass, ShouldEqual, string(ActionEnter))
					So(evaluation.Confidence, ShouldBeGreaterThan, 0.5)
				})
			})
		})

		Convey("When sufficient profitable trades accumulate to prove a net-positive edge", func() {
			for index := 0; index < 12; index++ {
				mainAgent.recordOutcome(TradeOutcome{
					Symbol:   "BTC/USD",
					Profit:   decimal.NewFromInt64(50),
					ReturnBp: 50.0,
					EntryAt:  time.Now().Add(-time.Minute),
					ExitAt:   time.Now(),
				}, 50.0)
				mainAgent.wins++
			}
			mainAgent.realized = decimal.NewFromInt64(600)
			mainAgent.evaluateRobustness()

			So(mainAgent.Status(), ShouldEqual, "trading")

			telemetryPromoted := mainAgent.AgentTelemetry()
			So(telemetryPromoted.Status, ShouldEqual, "trading")
			So(telemetryPromoted.Reading.Defined, ShouldBeTrue)
			So(telemetryPromoted.Reading.Mean, ShouldBeGreaterThan, 0)
			So(telemetryPromoted.Reading.Samples, ShouldEqual, 12)
		})

		Convey("When a weak or sparse precursor signal arrives", func() {
			weakAgent := NewMainAgent(initialCash, "paper", nil, priceSvc, nil)
			envelope := &types.Envelope{
				TickerData: kraken.TickerData{
					Symbol: "ETH/USD",
					Last:   decimal.NewFromInt64(3000),
				},
			}
			// Action is wait, not an entry signal
			sparseDecision := ActionDecision{
				Action:     ActionWait,
				Confidence: 0.22,
				Contrast:   0.3,
				Support:    1,
			}
			weakAgent.Step(envelope, sparseDecision)

			So(len(weakAgent.positions), ShouldEqual, 0)
			So(weakAgent.fills, ShouldEqual, 0)
		})

		Convey("When a long position is held and tick noise arrives", func() {
			noisyAgent := NewMainAgent(initialCash, "paper", nil, priceSvc, nil)
			price := decimal.NewFromInt64(100)
			envelope := &types.Envelope{
				TickerData: kraken.TickerData{Symbol: "SOL/USD", Last: price},
			}
			// Strong entry
			noisyAgent.Step(envelope, ActionDecision{
				Action:     ActionEnter,
				Confidence: 0.75,
				Contrast:   1.5,
				Support:    10,
			})
			So(len(noisyAgent.positions), ShouldEqual, 1)

			// Single-tick noise: wait with support = 1, contrast = 0.1
			noiseDecision := ActionDecision{
				Action:     ActionWait,
				Confidence: 0.35,
				Contrast:   0.1,
				Support:    1,
			}
			noisyAgent.Step(envelope, noiseDecision)

			// Position MUST remain open: not dumped on noise
			So(len(noisyAgent.positions), ShouldEqual, 1)

			// Wait while holding: keep holding
			holdDecision := ActionDecision{
				Action:     ActionWait,
				Confidence: 0.65,
				Contrast:   1.2,
				Support:    8,
			}
			noisyAgent.Step(envelope, holdDecision)
			So(len(noisyAgent.positions), ShouldEqual, 1)

			// Explicit exit signal: exit
			exitDecision := ActionDecision{
				Action:     ActionExit,
				Confidence: 0.70,
				Contrast:   1.4,
				Support:    9,
			}
			noisyAgent.Step(envelope, exitDecision)
			So(len(noisyAgent.positions), ShouldEqual, 0)
			So(noisyAgent.fills, ShouldEqual, 2)
		})

		Convey("When forward-tested edge turns negative, new simulated entries are halted", func() {
			lossAgent := NewMainAgent(initialCash, "paper", nil, priceSvc, nil)
			// Record 20 losing outcomes so samples >= 20 and meanReturn < 0
			for index := 0; index < 20; index++ {
				lossAgent.recordOutcome(TradeOutcome{
					Symbol:   "BTC/USD",
					Profit:   decimal.NewFromInt64(-50),
					ReturnBp: -50.0,
					EntryAt:  time.Now().Add(-time.Minute),
					ExitAt:   time.Now(),
				}, -50.0)
				lossAgent.losses++
			}
			lossAgent.realized = decimal.NewFromInt64(-1000)

			So(lossAgent.meanReturn, ShouldBeLessThan, 0)
			So(lossAgent.Status(), ShouldEqual, "learning")

			tel := lossAgent.AgentTelemetry()
			So(tel.Status, ShouldEqual, "learning")

			// Even with a high-conviction precursor signal, entry is blocked while edge is negative
			envelope := &types.Envelope{
				TickerData: kraken.TickerData{
					Symbol: "BTC/USD",
					Last:   decimal.NewFromInt64(50000),
				},
			}
			strongDecision := ActionDecision{
				Action:     ActionEnter,
				Confidence: 0.90,
				Contrast:   2.5,
				Support:    20,
			}
			lossAgent.Step(envelope, strongDecision)

			So(len(lossAgent.positions), ShouldEqual, 0)
			So(lossAgent.fills, ShouldEqual, 0)
		})
	})
}
