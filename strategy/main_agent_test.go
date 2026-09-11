package strategy

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func TestMainAgent(t *testing.T) {
	Convey("Given a freshly initialized MainAgent", t, func() {
		initialCash := decimal.NewFromInt64(10000)
		mainAgent := NewMainAgent(initialCash, "paper")

		So(mainAgent.ID(), ShouldEqual, 0)
		So(mainAgent.Status(), ShouldEqual, "simulated")
		So(mainAgent.TargetAccount(), ShouldEqual, "paper")

		telemetry := mainAgent.AgentTelemetry()
		So(telemetry.Initial, ShouldStartWith, "10000")
		So(telemetry.Cash, ShouldStartWith, "10000")
		So(telemetry.Equity, ShouldStartWith, "10000")
		So(telemetry.Fills, ShouldEqual, 0)
		So(telemetry.Decisions, ShouldEqual, 0)

		Convey("When an upward precursor signal arrives with high confidence", func() {
			price50k := decimal.NewFromInt64(50000)
			envelope := &types.Envelope{
				TickerData: kraken.TickerData{
					Symbol: "BTC/USD",
					Last:   price50k,
				},
			}
			consensus := PrecursorConsensus{
				Action:     "enter_long",
				Confidence: 0.85,
				Contrast:   2.4,
				Support:    12,
			}

			mainAgent.Step(envelope, consensus)

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

				mainAgent.Step(envelopeHigher, PrecursorConsensus{Action: "hold_long", Confidence: 0.6})

				So(mainAgent.unrealized.Sign(), ShouldBeGreaterThan, 0)
				So(mainAgent.equity.Cmp(initialCash), ShouldBeGreaterThan, 0)

				Convey("When a downward precursor signal triggers an exit", func() {
					envelopeExit := &types.Envelope{
						TickerData: kraken.TickerData{
							Symbol: "BTC/USD",
							Last:   price55k,
						},
					}
					downConsensus := PrecursorConsensus{
						Action:     "exit_long",
						Confidence: 0.80,
						Contrast:   1.8,
						Support:    15,
					}

					mainAgent.Step(envelopeExit, downConsensus)

					So(mainAgent.fills, ShouldEqual, 2)
					So(len(mainAgent.positions), ShouldEqual, 0)
					So(mainAgent.wins, ShouldEqual, 1)
					So(mainAgent.realized.Sign(), ShouldBeGreaterThan, 0)
					So(len(mainAgent.outcomes), ShouldEqual, 1)
					So(mainAgent.outcomes[0].ReturnBp, ShouldBeGreaterThan, 0)
				})
			})
		})

		Convey("When sufficient profitable trades accumulate to prove a net-positive edge", func() {
			for i := 0; i < 12; i++ {
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
			weakAgent := NewMainAgent(initialCash, "paper")
			envelope := &types.Envelope{
				TickerData: kraken.TickerData{
					Symbol: "ETH/USD",
					Last:   decimal.NewFromInt64(3000),
				},
			}
			// Action is wait, not an entry signal
			sparseConsensus := PrecursorConsensus{
				Action:     "wait",
				Confidence: 0.22,
				Contrast:   0.3,
				Support:    1,
			}
			weakAgent.Step(envelope, sparseConsensus)

			So(len(weakAgent.positions), ShouldEqual, 0)
			So(weakAgent.fills, ShouldEqual, 0)
		})

		Convey("When a long position is held and tick noise arrives", func() {
			noisyAgent := NewMainAgent(initialCash, "paper")
			price := decimal.NewFromInt64(100)
			envelope := &types.Envelope{
				TickerData: kraken.TickerData{Symbol: "SOL/USD", Last: price},
			}
			// Strong entry
			noisyAgent.Step(envelope, PrecursorConsensus{
				Action:     "enter_long",
				Confidence: 0.75,
				Contrast:   1.5,
				Support:    10,
			})
			So(len(noisyAgent.positions), ShouldEqual, 1)

			// Single-tick noise: wait with support = 1, contrast = 0.1
			noiseConsensus := PrecursorConsensus{
				Action:     "wait",
				Confidence: 0.35,
				Contrast:   0.1,
				Support:    1,
			}
			noisyAgent.Step(envelope, noiseConsensus)

			// Position MUST remain open: not dumped on noise
			So(len(noisyAgent.positions), ShouldEqual, 1)

			// Momentum continuation: hold_long
			holdConsensus := PrecursorConsensus{
				Action:     "hold_long",
				Confidence: 0.65,
				Contrast:   1.2,
				Support:    8,
			}
			noisyAgent.Step(envelope, holdConsensus)
			So(len(noisyAgent.positions), ShouldEqual, 1)

			// Explicit exit signal: exit_long
			exitConsensus := PrecursorConsensus{
				Action:     "exit_long",
				Confidence: 0.70,
				Contrast:   1.4,
				Support:    9,
			}
			noisyAgent.Step(envelope, exitConsensus)
			So(len(noisyAgent.positions), ShouldEqual, 0)
			So(noisyAgent.fills, ShouldEqual, 2)
		})

		Convey("When forward-tested edge turns negative, new simulated entries are halted", func() {
			lossAgent := NewMainAgent(initialCash, "paper")
			// Record 20 losing outcomes so samples >= 20 and meanReturn < 0
			for i := 0; i < 20; i++ {
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
			strongConsensus := PrecursorConsensus{
				Action:     "enter_long",
				Confidence: 0.90,
				Contrast:   2.5,
				Support:    20,
			}
			lossAgent.Step(envelope, strongConsensus)

			So(len(lossAgent.positions), ShouldEqual, 0)
			So(lossAgent.fills, ShouldEqual, 0)
		})
	})
}

