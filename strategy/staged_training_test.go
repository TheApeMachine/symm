package strategy

import (
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/broker/position"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/tests/market"
	"github.com/theapemachine/symm/tests/venue"
)

func TestStagedTrainingProgression(t *testing.T) {
	Convey("Staged training enforces explicit backend stages and skill gates", t, func() {
		ctx := t.Context()
		price := market.TrainingPrice(ctx)
		training := NewTraining(ctx, 1, price)
		training.Transition(runtime.READY)

		Convey("A fresh model begins in historical model development without trading authority", func() {
			stage, blocker := training.Stage()
			So(stage, ShouldEqual, StageModelDevelopment)
			So(blocker, ShouldNotBeBlank)

			// Replay alone cannot create paper trades
			trader := NewTrader(ctx, nil, price, nil)
			training.SetTrader(trader)

			frames := market.TrainingTape(4)
			for _, frame := range frames {
				training.Step(frame)
			}

			So(trader.PositionCount(), ShouldEqual, 0)
			So(len(trader.Positions()), ShouldEqual, 0)
		})

		Convey("Insufficient historical skill cannot enter paper-learning stage", func() {
			// Simulate historical reading with negative return
			training.Rehearsal.readingMu.Lock()
			training.Rehearsal.reading.Resolved = 10
			training.Rehearsal.reading.ValidUpOpportunities = 2
			training.Rehearsal.reading.Entered = 5
			training.Rehearsal.reading.Return = -0.05
			training.Rehearsal.reading.ReturnSq = 0.01
			training.Rehearsal.reading.MeanReturn = -0.01
			training.Rehearsal.reading.ReturnSE = 0.02
			training.Rehearsal.reading.LowerBound = -0.03
			training.Rehearsal.reading.ContinuationWaitCorrect = 3
			training.Rehearsal.reading.CorrectExit = 2
			training.Rehearsal.readingMu.Unlock()

			training.CheckStageGates()
			stage, blocker := training.Stage()
			So(stage, ShouldEqual, StageHistoricalValidation)
			So(blocker, ShouldContainSubstring, "historical return uncertainty spans zero")
		})

		Convey("Demonstrated historical skill transitions to forward paper learning", func() {
			training.Rehearsal.readingMu.Lock()
			training.Rehearsal.reading.Resolved = 20
			training.Rehearsal.reading.ValidUpOpportunities = 5
			training.Rehearsal.reading.Entered = 5
			training.Rehearsal.reading.Return = 0.20
			training.Rehearsal.reading.ReturnSq = 0.01
			training.Rehearsal.reading.MeanReturn = 0.04
			training.Rehearsal.reading.ReturnSE = 0.01
			training.Rehearsal.reading.LowerBound = 0.03 // strictly positive!
			training.Rehearsal.reading.ContinuationWaitCorrect = 5
			training.Rehearsal.reading.CorrectExit = 4
			training.Rehearsal.readingMu.Unlock()

			training.CheckStageGates()
			stage, blocker := training.Stage()
			So(stage, ShouldEqual, StageForwardPaperLearning)
			So(blocker, ShouldContainSubstring, "waiting for forward paper trades")
		})

		Convey("Forward skill demonstrated gate evaluates measured uncertainty without arbitrary sample caps", func() {
			training.SetStage(StageForwardPaperLearning, "")

			// Two profitable paper trades with positive variance and lower bound
			training.RecordForwardPaperTrade("BTC/USD", 0.05, 0.001)
			training.RecordForwardPaperTrade("BTC/USD", 0.04, 0.001)

			training.stageMu.Lock()
			training.forwardReading.CorrectWaitDown = 3
			training.forwardReading.CorrectExit = 2
			training.stageMu.Unlock()

			training.CheckStageGates()
			stage, _ := training.Stage()
			So(stage, ShouldEqual, StageForwardSkillDemonstrated)
		})

		Convey("Historical and forward counters remain strictly separate", func() {
			training.RecordForwardPaperTrade("BTC/USD", 0.05, 0.001)

			fwd := training.ForwardReading()
			So(fwd.PaperTrades, ShouldEqual, 1)
			So(fwd.PaperReturn, ShouldEqual, 0.05)

			// Rehearsal reading remains unchanged by forward trades
			hist := training.Rehearsal.reading
			So(hist.Entered, ShouldNotEqual, fwd.PaperTrades)
		})
	})
}

func TestForwardPaperLearning(t *testing.T) {
	Convey("Forward paper learning scores frozen predictions before refinement and tracks execution", t, func() {
		ctx := t.Context()
		viper.Set("trading.allocation.max_fraction", 0.5)
		viper.Set("market.quote_currency", "USD")

		conn := venue.NewConn()
		conn.BalanceResult = kraken.NewBalance([]byte(`{
			"channel": "balances",
			"type": "snapshot",
			"data": [{"asset": "USD", "balance": 1000.0}]
		}`))

		api := websocket.NewAPI(ctx, conn, conn, &websocket.FuturesLive{})
		api.Normalizer().Update(&spot.AssetsManagerUpdate{
			NewAssets: map[string]spot.AssetInfo{
				"BTC": {AltName: "BTC", Decimals: 8, DisplayDecimals: 8},
				"USD": {AltName: "USD", Decimals: 2, DisplayDecimals: 2},
			},
			NewPairs: map[string]spot.AssetPair{
				"BTCUSD": {
					WSName: "BTC/USD", Base: "BTC", Quote: "USD",
					PairDecimals: 2, LotDecimals: 8, LotMultiplier: 1,
				},
			},
		})
		api.Transition(runtime.READY)

		instrument := broker.NewInstrument(api)
		price := broker.NewPrice(ctx, api, instrument)
		price.SetFee("BTC/USD", kraken.TradeVolumeFee{
			Fee: decimal.NewFromFloat64(0.001),
		})
		price.Update(&kraken.TickerData{
			Symbol: "BTC/USD",
			Ask:    decimal.NewFromFloat64(50000.0),
			Bid:    decimal.NewFromFloat64(49950.0),
		})
		price.Transition(runtime.READY)

		balance := broker.NewBalance(ctx, api)
		trader := NewTrader(ctx, api, price, balance)

		training := NewTraining(ctx, 1, price)
		training.Transition(runtime.READY)
		training.SetTrader(trader)

		// Authorize forward paper learning stage
		training.SetStage(StageForwardPaperLearning, "")

		// 1. Live model emits a prediction and freezes it before outcome arrives
		testTokens := []uint64{101, 102}
		training.precursor.SetTokens("BTC/USD", testTokens)
		contextKey := EncodeTokens("BTC/USD", false, testTokens)

		training.freezeForwardPrediction("BTC/USD", contextKey, testTokens, 100, ActionEnter, cognition.Evaluation{
			Support:    10,
			Confidence: 0.85,
			Ambiguity:  0.10,
		})

		// 2. Unresolved prediction does not train the outcome yet
		initialCensus := training.engine.Census()

		// 3. Live market fragment completes with negative outcome (DOWN tape)
		resolvedRecord := &tables.ExcursionRecord{
			Symbol:         "BTC/USD",
			Direction:      "DOWN",
			AnchorTick:     100,
			ExitTick:       110,
			ProfitFraction: -0.03,
			ClearsFriction: false,
		}

		training.resolveForwardOutcome(resolvedRecord)

		// 4. Frozen prediction was scored: false enter on DOWN was counted
		fwd := training.ForwardReading()
		So(fwd.EnterPredictions, ShouldEqual, 1)
		So(fwd.FalseEnterDown, ShouldEqual, 1)
		So(fwd.CorrectEnter, ShouldEqual, 0)

		// 5. Only after scoring, newly resolved example trained the trie (ActionWait on DOWN)
		postCensus := training.engine.Census()
		So(postCensus["wait"], ShouldBeGreaterThan, initialCensus["wait"])

		// 6. Complete paper trade execution path:
		// Training action -> Trader.OnAction -> Desk -> paper AddOrder -> paper execution callback -> Regulator -> close callback
		system.Cfg.Market.Model = "paper"
		trader.SetOnPositionClosed(training.RecordForwardPaperTrade)

		// Model selects ENTER: executes via Desk.EnterWithRegulator and places paper AddOrder
		trader.OnAction("BTC/USD", ActionEnter)

		posVal, found := trader.positions.Load("BTC/USD")
		So(found, ShouldBeTrue)
		reg := posVal.(*position.Regulator)
		So(reg.Pending, ShouldNotBeNil)
		So(reg.Pending.Type, ShouldEqual, "buy")

		// Paper execution callback arrives from venue
		enterExec := &kraken.Execution{
			Channel: "executions",
			Type:    "update",
			Data: []kraken.ExecutionData{
				{
					Symbol:        "BTC/USD",
					OrderID:       "order-1",
					ClientOrderID: reg.PositionID,
					OrderStatus:   "filled",
					AvgPrice:      decimal.NewFromFloat64(50000.0),
					CumQty:        decimal.NewFromFloat64(0.001),
					CumCost:       decimal.NewFromFloat64(50.0),
					FeeUsdEquiv:   decimal.NewFromFloat64(0.10),
				},
			},
		}
		conn.EmitExecution(enterExec)
		So(trader.Holding("BTC/USD"), ShouldBeTrue)
		So(trader.HasFilledPosition("BTC/USD"), ShouldBeTrue)

		// Model selects EXIT: executes via Desk.Exit and places paper AddOrder
		trader.OnAction("BTC/USD", ActionExit)
		So(reg.Pending, ShouldNotBeNil)
		So(reg.Pending.Type, ShouldEqual, "sell")
		exitClOrdId := reg.Pending.ClOrdId

		exitExec := &kraken.Execution{
			Channel: "executions",
			Type:    "update",
			Data: []kraken.ExecutionData{
				{
					Symbol:        "BTC/USD",
					OrderID:       "order-2",
					ClientOrderID: exitClOrdId,
					OrderStatus:   "filled",
					AvgPrice:      decimal.NewFromFloat64(52000.0),
					CumQty:        decimal.NewFromFloat64(0.001),
					CumCost:       decimal.NewFromFloat64(52.0),
					FeeUsdEquiv:   decimal.NewFromFloat64(0.10),
				},
			},
		}
		conn.EmitExecution(exitExec)
		So(trader.Holding("BTC/USD"), ShouldBeFalse)

		// Authoritative close callback fired from ApplyExecution and recorded forward paper trade!
		fwd = training.ForwardReading()
		So(fwd.PaperTrades, ShouldEqual, 1)
		So(fwd.PaperProfitable, ShouldEqual, 1)
		So(fwd.PaperReturn, ShouldAlmostEqual, 0.0359, 1e-3)
		So(fwd.PaperFees, ShouldAlmostEqual, 0.20, 1e-3)

		// Forward stats contain no historical samples
		So(training.Rehearsal.reading.Entered, ShouldEqual, 0)
	})
}
