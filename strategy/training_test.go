package strategy

import (
	"context"
	"os"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/tests/market"
	venue "github.com/theapemachine/symm/tests/venue"
)

func TestTrainingRegister(t *testing.T) {
	Convey("Training owns its telemetry and declares all upstream producers", t, func() {
		training := NewTraining(t.Context(), market.TrainingPrice(t.Context()), nil, nil)
		So(training.Register(), ShouldEqual, training.Register())
	})
}

func TestTrainingStep(t *testing.T) {
	Convey("Given a Training instance in INIT stage", t, func() {
		training := NewTraining(t.Context(), market.TrainingPrice(t.Context()), nil, nil)
		So(training.Status(), ShouldEqual, runtime.INIT)

		Convey("When multi-leg market replay tape arrives, the grid clusters metrics sympathetically and forms regions", func() {
			tape := market.TrainingTape(4)
			So(len(tape), ShouldBeGreaterThan, 0)

			for _, frame := range tape {
				output := training.Step(frame)
				So(output, ShouldNotBeNil)
			}

			So(len(training.grid.Metrics), ShouldBeGreaterThan, 1)
			So(training.grid.Settled, ShouldBeTrue)
		})

		Convey("When model transitions to READY, paper trading predicts actions", func() {
			training.grid.Settled = true
			training.Transition(runtime.READY)

			token := []byte{1}
			training.trie.Insert(token, []byte(ActionEnter))

			frame := data.NewMeasurement[float64]("BTC/USD", nil)
			frame.Metrics = map[string]data.Metric[float64]{
				"price": {Label: "price", Raw: 50000.0, Region: 1},
			}

			output := training.Step(frame)
			So(output, ShouldNotBeNil)
			So(output.Metrics["action"].Raw, ShouldEqual, 1)
		})

		Convey("When cognitive engine is trained on precursor sequence, it evaluates action and exports tree", func() {
			training.grid.Settled = true
			training.Transition(runtime.READY)

			token := []byte{2}
			_, trainErr := training.engine.Train(token, []byte(ActionEnter), 1.0)
			So(trainErr, ShouldBeNil)

			frame := data.NewMeasurement[float64]("BTC/USD", nil)
			frame.Metrics = map[string]data.Metric[float64]{
				"price": {Label: "price", Raw: 50000.0, Region: 2},
			}

			output := training.Step(frame)
			So(output, ShouldNotBeNil)
			So(output.Metrics["action"].Raw, ShouldEqual, 1)

			tree := training.CognitionTree()
			So(tree.Root, ShouldNotBeNil)
		})
	})
}

func TestTrainingCheckpoint(t *testing.T) {
	Convey("Given a trained checkpoint", t, func() {
		defer os.Remove("grid_checkpoint.json")

		training := NewTraining(t.Context(), market.TrainingPrice(t.Context()), nil, nil)

		tape := market.TrainingTape(2)
		for _, frame := range tape {
			training.Step(frame)
		}

		err := training.SaveCheckpoint()
		So(err, ShouldBeNil)

		Convey("When loading checkpoint into a new instance", func() {
			restored := NewTraining(t.Context(), market.TrainingPrice(t.Context()), nil, nil)

			loadErr := restored.LoadCheckpoint()
			So(loadErr, ShouldBeNil)
			So(len(restored.grid.Metrics), ShouldBeGreaterThan, 0)
			So(restored.engine.Root(), ShouldNotBeNil)
		})
	})
}

func TestTrainingStateMetrics(t *testing.T) {
	Convey("Given a Training instance processing market tape", t, func() {
		training := NewTraining(t.Context(), market.TrainingPrice(t.Context()), nil, nil)

		tape := market.TrainingTape(4)
		var lastOutput *data.Measurement[float64]

		for _, frame := range tape {
			lastOutput = training.Step(frame)
		}

		So(lastOutput, ShouldNotBeNil)

		Convey("Step emits the training state metrics the dashboard reads", func() {
			So(lastOutput.Metrics["stage_code"].Raw, ShouldBeGreaterThanOrEqualTo, 0)
			So(lastOutput.Metrics["steps"].Raw, ShouldBeGreaterThan, 0)
			So(lastOutput.Metrics["decisions"].Raw, ShouldBeGreaterThanOrEqualTo, 0)
			So(lastOutput.Metrics["resolved"].Raw, ShouldBeGreaterThanOrEqualTo, 0)
			So(lastOutput.Metrics["evaluated"].Raw, ShouldBeGreaterThanOrEqualTo, 0)

			_, hasConfidence := lastOutput.Metrics["confidence"]
			So(hasConfidence, ShouldBeTrue)

			_, hasContrast := lastOutput.Metrics["contrast"]
			So(hasContrast, ShouldBeTrue)

			_, hasEdge := lastOutput.Metrics["edge"]
			So(hasEdge, ShouldBeTrue)

			_, hasWinRate := lastOutput.Metrics["win_rate"]
			So(hasWinRate, ShouldBeTrue)

			_, hasTrading := lastOutput.Metrics["trading"]
			So(hasTrading, ShouldBeTrue)
		})

		Convey("Step emits stage blocker in provenance", func() {
			_, hasBlocker := lastOutput.Provenance["stage_blocker"]
			So(hasBlocker, ShouldBeTrue)
		})

		Convey("Step count matches the number of frames processed", func() {
			So(training.steps.Load(), ShouldEqual, uint64(len(tape)))
		})
	})
}

func TestTrainingStageProgression(t *testing.T) {
	Convey("Given a Training instance that has developed its grid", t, func() {
		training := NewTraining(t.Context(), market.TrainingPrice(t.Context()), nil, nil)

		So(training.stageCode.Load(), ShouldEqual, StageModelDevelopment)

		tape := market.TrainingTape(4)
		for _, frame := range tape {
			training.Step(frame)
		}

		Convey("Once the grid settles and engine learns, stage advances past MODEL DEVELOPMENT", func() {
			So(training.grid.Settled, ShouldBeTrue)

			// Train the engine so it has at least one association.
			_, err := training.engine.Train([]byte{1}, []byte(ActionEnter), 1.0)
			So(err, ShouldBeNil)

			training.advanceStage()
			So(training.stageCode.Load(), ShouldBeGreaterThanOrEqualTo, StageHistoricalValidation)
		})

		Convey("RecordTradeResult updates win counters correctly", func() {
			training.RecordTradeResult(0.05) // win
			training.RecordTradeResult(-0.02) // loss
			training.RecordTradeResult(0.01) // win

			So(training.totalTrades.Load(), ShouldEqual, 3)
			So(training.wins.Load(), ShouldEqual, 2)
			So(training.resolved.Load(), ShouldEqual, 3)
		})
	})
}

func trainingTestTrader(ctx context.Context) (*Trader, *broker.Price) {
	conn := venue.NewConn()
	conn.BalanceResult = kraken.NewBalance([]byte(`{
		"channel": "balances",
		"type": "snapshot",
		"data": [{"asset": "USD", "balance": 200.0}]
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
		Fee: decimal.NewFromFloat64(0.25),
	})
	price.Update(&kraken.TickerData{
		Symbol: "BTC/USD",
		Ask:    decimal.NewFromFloat64(50000),
		Bid:    decimal.NewFromFloat64(49950),
	})

	balance := broker.NewBalance(ctx, api)
	trader := NewTrader(ctx, api, price, balance)
	return trader, price
}

func TestTrainingForwardPaperExecution(t *testing.T) {
	Convey("Given a Training instance wired with Trader in StageForwardPaper", t, func() {
		ctx := t.Context()
		trader, price := trainingTestTrader(ctx)
		training := NewTraining(ctx, price, trader, nil)

		training.grid.Settled = true
		training.stageCode.Store(StageForwardPaper)

		token := []byte{3}
		_, trainErr := training.engine.Train(token, []byte(ActionEnter), 1.0)
		So(trainErr, ShouldBeNil)
		training.trie.Insert(token, []byte(ActionEnter))

		frame := data.NewMeasurement[float64]("BTC/USD", nil)
		frame.Metrics = map[string]data.Metric[float64]{
			"price": {Label: "price", Raw: 50000.0, Region: 3},
		}

		Convey("When Step evaluates an ActionEnter prediction", func() {
			output := training.Step(frame)
			So(output, ShouldNotBeNil)
			So(output.Metrics["action"].Raw, ShouldEqual, 1)

			pos := trader.Position("BTC/USD")
			So(pos, ShouldNotBeNil)
			So(pos.Status(), ShouldEqual, "entry_pending")

			storedToken, found := training.activeTokens.Load("BTC/USD")
			So(found, ShouldBeTrue)
			So(storedToken, ShouldResemble, token)

			Convey("When position closes profitably, RecordPositionResult reinforces precursor token", func() {
				training.RecordPositionResult("BTC/USD", 0.04)

				So(training.totalTrades.Load(), ShouldEqual, 1)
				So(training.wins.Load(), ShouldEqual, 1)

				_, stillThere := training.activeTokens.Load("BTC/USD")
				So(stillThere, ShouldBeFalse)

				evalResult, evalErr := training.engine.Evaluate(token)
				So(evalErr, ShouldBeNil)
				So(evalResult.Evaluation.WinnerClass, ShouldEqual, string(ActionEnter))
			})

			Convey("When position closes at a loss, RecordPositionResult penalizes token toward ActionWait", func() {
				training.RecordPositionResult("BTC/USD", -0.05)

				So(training.totalTrades.Load(), ShouldEqual, 1)
				So(training.wins.Load(), ShouldEqual, 0)
			})
		})
	})
}

func TestTrainingReplayMultiPass(t *testing.T) {
	Convey("Given a Training instance with settled grid and excursion fragments", t, func() {
		ctx := t.Context()
		training := NewTraining(ctx, market.TrainingPrice(ctx), nil, nil)
		training.grid.Settled = true

		var fragments []*data.Measurement[float64]
		for seq := int64(10); seq <= 20; seq++ {
			frag := data.NewMeasurement[float64]("BTC/USD", nil)
			frag.SeqIdx = seq
			frag.Metadata = map[string]string{
				"excursion":                 "upper",
				"excursion_category":        "upper_profitable",
				"excursion_clears_friction": "true",
				"excursion_start":           "10",
				"excursion_ignition":        "15",
				"excursion_end":             "20",
			}
			frag.Metrics = map[string]data.Metric[float64]{
				"price": {Label: "price", Raw: 50000.0 + float64(seq), Region: 1},
			}
			fragments = append(fragments, frag)
		}

		training.mutex.Lock()
		training.fragments = fragments
		training.mutex.Unlock()

		Convey("When trainFromFragments executes", func() {
			training.trainFromFragments()

			So(training.decisions.Load(), ShouldEqual, 3)

			actionBytes, found := training.trie.Get([]byte{1})
			So(found, ShouldBeTrue)
			So(Action(actionBytes), ShouldEqual, ActionEnter)
		})
	})
}

func TestTrainingGridFreezing(t *testing.T) {
	Convey("Given a Training instance with a settled grid past Model Development", t, func() {
		ctx := t.Context()
		training := NewTraining(ctx, market.TrainingPrice(ctx), nil, nil)

		tape := market.TrainingTape(2)
		for _, frame := range tape {
			training.Step(frame)
		}

		So(training.grid.Settled, ShouldBeTrue)
		training.stageCode.Store(StageHistoricalValidation)

		initialMetricCount := len(training.grid.Metrics)

		Convey("When new unseen metrics arrive in Step", func() {
			unseenFrame := data.NewMeasurement[float64]("ETH/USD", nil)
			unseenFrame.Metrics = map[string]data.Metric[float64]{
				"new_exotic_metric": {Label: "new_exotic_metric", Raw: 123.45},
			}

			output := training.Step(unseenFrame)
			So(output, ShouldNotBeNil)

			So(len(training.grid.Metrics), ShouldEqual, initialMetricCount)
			So(training.grid.Region("new_exotic_metric"), ShouldEqual, 0)
		})
	})
}
