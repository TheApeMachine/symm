package strategy

import (
	"sync"
	"testing"
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/types"
)

type testBookSource struct {
	books sync.Map
}

func (source *testBookSource) Book(symbol string, read func(*spotbook.Book)) {
	if val, ok := source.books.Load(symbol); ok {
		read(val.(*spotbook.Book))
	}
}

func (source *testBookSource) SetTouch(symbol string, bidPrice, bidQty, askPrice, askQty *decimal.Decimal) {
	book := spotbook.New()
	book.Update(&spotbook.UpdateOptions{
		Direction: spotbook.Bid,
		ID:        "1",
		Price:     bidPrice,
		Quantity:  bidQty,
	})
	book.Update(&spotbook.UpdateOptions{
		Direction: spotbook.Ask,
		ID:        "2",
		Price:     askPrice,
		Quantity:  askQty,
	})
	source.books.Store(symbol, book)
}

func testPrice() *broker.Price {
	instrument := broker.NewInstrumentWithQuote("USD")
	price := broker.NewPrice(nil, instrument)
	feeRate := decimal.NewFromFloat64(0.26)
	price.SetFee("BTC/USD", kraken.TradeVolumeFee{Fee: feeRate})
	price.SetFee("ETH/USD", kraken.TradeVolumeFee{Fee: feeRate})
	price.SetFee("SOL/USD", kraken.TradeVolumeFee{Fee: feeRate})

	return price
}

func testExecutablePrice() (*broker.Price, *testBookSource) {
	instrument := broker.NewInstrumentWithQuote("USD")
	price := broker.NewPrice(nil, instrument)
	feeRate := decimal.NewFromFloat64(0.26)
	price.SetFee("BTC/USD", kraken.TradeVolumeFee{Fee: feeRate})
	price.SetFee("ETH/USD", kraken.TradeVolumeFee{Fee: feeRate})
	price.SetFee("SOL/USD", kraken.TradeVolumeFee{Fee: feeRate})

	if price.Normalizer() != nil {
		price.Normalizer().Update(&spot.AssetsManagerUpdate{
			NewAssets: map[string]spot.AssetInfo{
				"BTC": {AltName: "BTC", Decimals: 8, DisplayDecimals: 8},
				"ETH": {AltName: "ETH", Decimals: 8, DisplayDecimals: 8},
				"SOL": {AltName: "SOL", Decimals: 8, DisplayDecimals: 8},
				"USD": {AltName: "USD", Decimals: 2, DisplayDecimals: 2},
			},
			NewPairs: map[string]spot.AssetPair{
				"BTCUSD": {WSName: "BTC/USD", Base: "BTC", Quote: "USD", PairDecimals: 2, LotDecimals: 8, LotMultiplier: 1},
				"ETHUSD": {WSName: "ETH/USD", Base: "ETH", Quote: "USD", PairDecimals: 2, LotDecimals: 8, LotMultiplier: 1},
				"SOLUSD": {WSName: "SOL/USD", Base: "SOL", Quote: "USD", PairDecimals: 2, LotDecimals: 8, LotMultiplier: 1},
			},
		})
	}

	books := &testBookSource{}
	books.SetTouch("BTC/USD", decimal.NewFromInt64(49990), decimal.NewFromInt64(10), decimal.NewFromInt64(50000), decimal.NewFromInt64(10))
	books.SetTouch("ETH/USD", decimal.NewFromInt64(2990), decimal.NewFromInt64(100), decimal.NewFromInt64(3000), decimal.NewFromInt64(100))
	books.SetTouch("SOL/USD", decimal.NewFromInt64(95), decimal.NewFromInt64(1000), decimal.NewFromInt64(100), decimal.NewFromInt64(1000))
	price.Books = books

	return price, books
}

func TestMainAgent(t *testing.T) {
	Convey("Given a MainAgent operating with an associative cognition engine", t, func() {
		initialCash := decimal.NewFromInt64(1000)
		engine := cognition.NewEngine(cognition.Config{})
		priceSvc, books := testExecutablePrice()
		mainAgent := NewMainAgent(initialCash, "paper", nil, priceSvc, engine)

		So(mainAgent.ID(), ShouldEqual, 0)
		So(mainAgent.Status(), ShouldEqual, "paper")
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
				books.SetTouch("BTC/USD", decimal.NewFromInt64(54990), decimal.NewFromInt64(10), decimal.NewFromInt64(55000), decimal.NewFromInt64(10))
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

					// Forward-testing does not pollute shared cognition; only rehearsal workers reinforce memory
					evaluation := mustEvaluate(t, engine, entryCtx)
					So(evaluation.Support, ShouldEqual, 0)
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

		Convey("When edge turns negative, live trading execution is demoted to protect capital", func() {
			tradingAgent := NewMainAgent(initialCash, "paper", nil, priceSvc, nil)
			tradingAgent.status = "trading"
			// Record 20 losing outcomes so samples >= 20 and meanReturn < 0
			for index := 0; index < 20; index++ {
				tradingAgent.recordOutcome(TradeOutcome{
					Symbol:   "BTC/USD",
					Profit:   decimal.NewFromInt64(-50),
					ReturnBp: -50.0,
					EntryAt:  time.Now().Add(-time.Minute),
					ExitAt:   time.Now(),
				}, -50.0)
				tradingAgent.losses++
			}
			tradingAgent.realized = decimal.NewFromInt64(-1000)
			tradingAgent.evaluateRobustness()

			So(tradingAgent.meanReturn, ShouldBeLessThan, 0)
			So(tradingAgent.status, ShouldEqual, "paper")
			So(tradingAgent.Status(), ShouldEqual, "learning")

			// In simulated mode, forward testing continues evaluating high-conviction decisions
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
			tradingAgent.Step(envelope, strongDecision)

			So(len(tradingAgent.positions), ShouldEqual, 1)
			So(tradingAgent.fills, ShouldEqual, 1)
		})

		Convey("When fee is present but resident book is absent or insufficient, MainAgent takes zero fills and zero positions", func() {
			priceNoBook := testPrice()
			strictAgent := NewMainAgent(initialCash, "paper", nil, priceNoBook, engine)

			envelope := &types.Envelope{
				TickerData: kraken.TickerData{
					Symbol: "BTC/USD",
					Last:   decimal.NewFromInt64(50000),
				},
			}
			decision := ActionDecision{
				Action:     ActionEnter,
				Context:    []byte("entry_ctx"),
				Confidence: 0.85,
				Contrast:   2.4,
				Support:    12,
			}

			strictAgent.Step(envelope, decision)

			So(strictAgent.fills, ShouldEqual, 0)
			So(len(strictAgent.positions), ShouldEqual, 0)
			So(strictAgent.cash.Cmp(initialCash), ShouldEqual, 0)
		})
	})
}
