package strategy

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/tests/tablestest"
)

var testRegionPins = [12]struct {
	source string
	metric string
}{
	0:  {"hawkes", "buy_from_buy"},
	1:  {"hawkes", "branching_ratio"},
	2:  {"hawkes", "sell_from_sell"},
	3:  {"cvd", "quantity:buy"},
	4:  {"cvd", "churn"},
	5:  {"cvd", "quantity:sell"},
	6:  {"depthflow", "removed:ask"},
	7:  {"depthflow", "spread"},
	8:  {"depthflow", "removed:bid"},
	9:  {"sentiment", "advance_fraction"},
	10: {"morphology", "wasserstein"},
	11: {"sentiment", "decline_fraction"},
}

func TestTrainer_MCTSPrecursorWindowOptimization(t *testing.T) {
	Convey("1. Different precursor tick counts produce naturally varying token depths", t, func() {
		grid := store.NewGrid()
		tapeByTick := make(map[int64][]*data.Measurement)
		epoch := int64(100)
		symbol := "BTC/USD"
		now := time.Now().UTC()

		peers := make([]*data.Measurement, 12)
		for tick := int64(1); tick <= 50; tick++ {
			tickMeas := make([]*data.Measurement, 0, 12)
			for idx, pin := range testRegionPins {
				rawVal := float64(((tick + int64(idx)) / 3) + 1)
				if peers[idx] == nil {
					peers[idx] = data.NewMeasurement(epoch, symbol, pin.source, tick, tick)
					peers[idx].At = now.Add(time.Duration(tick) * time.Second)
					peers[idx].From = peers[idx].At
					peers[idx].Write(data.NewMetric(pin.metric, rawVal, data.UnitCount, data.TimescaleTick))
				} else {
					peers[idx] = peers[idx].Next(pin.source, map[string]float64{pin.metric: rawVal})
					peers[idx].At = now.Add(time.Duration(tick) * time.Second)
					peers[idx].From = peers[idx].At
				}
				tickMeas = append(tickMeas, peers[idx])
			}
			tapeByTick[tick] = tickMeas
		}

		bTick := int64(45)

		// Short window of 4 ticks (ticks 41..44)
		shortWindow := int64(4)
		shortTokens := tokenizeTape(grid, tapeByTick, bTick-shortWindow, bTick, false)

		// Longer window of 18 ticks (ticks 27..44)
		longWindow := int64(18)
		longTokens := tokenizeTape(grid, tapeByTick, bTick-longWindow, bTick, false)

		So(len(shortTokens), ShouldBeGreaterThan, 0)
		So(len(longTokens), ShouldBeGreaterThan, len(shortTokens))
	})

	Convey("2. Consecutive identical region tokens are collapsed", t, func() {
		grid := store.NewGrid()
		tapeByTick := make(map[int64][]*data.Measurement)
		epoch := int64(100)
		symbol := "BTC/USD"
		now := time.Now().UTC()

		peers := make([]*data.Measurement, 12)
		for tick := int64(1); tick <= 25; tick++ {
			tickMeas := make([]*data.Measurement, 0, 12)
			for idx, pin := range testRegionPins {
				rawVal := float64(2 * (tick % 2))
				if idx == 0 {
					rawVal += 10.0
				}
				if peers[idx] == nil {
					peers[idx] = data.NewMeasurement(epoch, symbol, pin.source, tick, tick)
					peers[idx].At = now.Add(time.Duration(tick) * time.Second)
					peers[idx].From = peers[idx].At
					peers[idx].Write(data.NewMetric(pin.metric, rawVal, data.UnitCount, data.TimescaleTick))
				} else {
					peers[idx] = peers[idx].Next(pin.source, map[string]float64{pin.metric: rawVal})
					peers[idx].At = now.Add(time.Duration(tick) * time.Second)
					peers[idx].From = peers[idx].At
				}
				tickMeas = append(tickMeas, peers[idx])
			}
			tapeByTick[tick] = tickMeas
		}

		tokens := tokenizeTape(grid, tapeByTick, 17, 25, false)
		So(len(tokens), ShouldEqual, 1)
	})

	Convey("3. Positive and negative action paths survive training without pruning", t, func() {
		trie := newPrecursorTrie()

		// Insert positive action path
		positivePath := []string{"R01", "R02", "R04"}
		trie.Insert(positivePath, actionEnter, 0.05, 0.06, 60.0)

		// Insert negative action path
		negativePath := []string{"R01", "R03", "R05"}
		trie.Insert(negativePath, actionNoop, -0.02, -0.01, 30.0)

		So(trie.ActionCount(actionEnter), ShouldEqual, 1)
		So(trie.ActionCount(actionNoop), ShouldEqual, 1)

		// Both exact paths exist and match without pruning
		So(trie.Consensus(positivePath), ShouldEqual, actionEnter)
		So(trie.Consensus(negativePath), ShouldEqual, actionNoop)
	})

	Convey("4. Conflicting matching continuations inhibit action", t, func() {
		trie := newPrecursorTrie()

		// Two paths sharing prefix R01/R02: one enters, one is noop (e.g. chop/failed excursion)
		trie.Insert([]string{"R01", "R02", "R03"}, actionEnter, 0.04, 0.05, 50.0)
		trie.Insert([]string{"R01", "R02", "R04"}, actionNoop, -0.03, -0.02, 40.0)

		// Querying prefix R01/R02 encounters disagreement: must return empty (abstain)
		action := trie.Consensus([]string{"R01", "R02"})
		So(action, ShouldEqual, "")
	})

	Convey("5. Unanimous continuations permit early action on incomplete prefixes", t, func() {
		trie := newPrecursorTrie()

		// Two distinct paths sharing prefix R01/R02, both ending in actionEnter
		trie.Insert([]string{"R01", "R02", "R03"}, actionEnter, 0.04, 0.05, 50.0)
		trie.Insert([]string{"R01", "R02", "R05"}, actionEnter, 0.06, 0.07, 70.0)

		// Querying the incomplete prefix R01/R02 should unanimously permit enter
		action := trie.Consensus([]string{"R01", "R02"})
		So(action, ShouldEqual, actionEnter)

		// Unobserved prefix returns empty (abstain)
		So(trie.Consensus([]string{"R99"}), ShouldEqual, "")
	})

	Convey("6. Training examples do not leak into their own predictive evaluation", t, func() {
		excursions := make([]*excursionData, 0)

		for idx := int64(1); idx <= 10; idx++ {
			bTick := idx * 10
			excursions = append(excursions, &excursionData{
				startTick: bTick - 5,
				bTick:     bTick,
				cTick:     bTick + 3,
			})
		}

		tapeTicks := make([]int64, 0)
		for tick := int64(1); tick <= 120; tick++ {
			tapeTicks = append(tapeTicks, tick)
		}

		train, eval, evalTicks := splitChronologically(excursions, tapeTicks)

		So(len(train), ShouldBeGreaterThan, 0)
		So(len(eval), ShouldBeGreaterThan, 0)
		So(len(train)+len(eval), ShouldEqual, len(excursions))

		// Maximum training bTick is strictly before the first evaluation startTick
		maxTrainBTick := train[len(train)-1].bTick
		minEvalStartTick := eval[0].startTick
		So(maxTrainBTick, ShouldBeLessThanOrEqualTo, minEvalStartTick)

		// Evaluation tape ticks only contain ticks in the evaluation horizon
		So(evalTicks[0], ShouldBeGreaterThanOrEqualTo, minEvalStartTick)
	})

	Convey("7. MCTS evaluates candidate models and selects according to observed replay results", t, func() {
		ctx := context.Background()
		normalizer := spot.NewNormalizer()
		normalizer.Update(&spot.AssetsManagerUpdate{
			NewAssets: map[string]spot.AssetInfo{
				"BTC": {AltName: "XBT"},
				"USD": {AltName: "USD"},
			},
			NewPairs: map[string]spot.AssetPair{
				"BTC/USD": {WSName: "BTC/USD", Base: "BTC", Quote: "USD", LotDecimals: 8, LotMultiplier: 1},
			},
		})

		price := broker.NewPrice(ctx, nil, nil, nil, normalizer)
		price.SetFee("BTC/USD", kraken.TradeVolumeFee{Fee: decimal.NewFromFloat64(0.26)})
		price.SetReferenceCash(decimal.NewFromFloat64(200))
		price.SetQuote("BTC/USD", decimal.NewFromFloat64(59990), decimal.NewFromFloat64(60000))

		trainer := NewTrainer(ctx, nil, price)
		grid := store.NewGrid()
		tapeByTick := make(map[int64][]*data.Measurement)
		epoch := int64(200)
		symbol := "BTC/USD"
		now := time.Now().UTC()

		for tick := int64(1); tick <= 100; tick++ {
			tickMeas := make([]*data.Measurement, 0, 12)
			for idx, pin := range testRegionPins {
				meas := data.NewMeasurement(epoch, symbol, pin.source, tick, tick)
				meas.At = now.Add(time.Duration(tick) * time.Second)
				meas.From = meas.At
				metric := data.NewMetric(pin.metric, float64(tick%5)+float64(idx), data.UnitCount, data.TimescaleTick)
				metric.Standardized = float64(tick%5) + float64(idx)
				meas.Write(metric)
				tickMeas = append(tickMeas, meas)
			}
			tapeByTick[tick] = tickMeas
		}

		trainExcursions := []*excursionData{
			{
				symbol:      symbol,
				epoch:       epoch,
				startTick:   10,
				bTick:       16,
				cTick:       18,
				bPrice:      decimal.NewFromFloat64(60000),
				cPrice:      decimal.NewFromFloat64(63000),
				isUp:        true,
				class:       excursionUp,
				gain:        0.05,
				netReturn:   0.04,
				holdSeconds: 2.0,
			},
		}

		evalExcursions := []*excursionData{
			{
				symbol:      symbol,
				epoch:       epoch,
				startTick:   40,
				bTick:       46,
				cTick:       48,
				bPrice:      decimal.NewFromFloat64(60000),
				cPrice:      decimal.NewFromFloat64(63000),
				isUp:        true,
				class:       excursionUp,
				gain:        0.05,
				netReturn:   0.04,
				holdSeconds: 2.0,
			},
		}

		evalTicks := make([]int64, 0)
		for tick := int64(30); tick <= 60; tick++ {
			evalTicks = append(evalTicks, tick)
		}

		selectedTicks, _ := trainer.optimizePrecursorTicks(
			trainExcursions,
			tapeByTick,
			evalExcursions,
			evalTicks,
			grid,
			2,
			10,
		)

		So(selectedTicks, ShouldBeGreaterThanOrEqualTo, 2)
		So(selectedTicks, ShouldBeLessThanOrEqualTo, 10)
	})

	Convey("8. End-to-end Generate: empirical MCTS selection and unpruned publication", t, func() {
		ctx := context.Background()
		catalog := tablestest.New(t)
		normalizer := spot.NewNormalizer()
		normalizer.Update(&spot.AssetsManagerUpdate{
			NewAssets: map[string]spot.AssetInfo{
				"BTC": {AltName: "XBT"},
				"USD": {AltName: "USD"},
			},
			NewPairs: map[string]spot.AssetPair{
				"BTC/USD": {WSName: "BTC/USD", Base: "BTC", Quote: "USD", LotDecimals: 8, LotMultiplier: 1},
			},
		})

		price := broker.NewPrice(ctx, nil, nil, nil, normalizer)
		price.SetFee("BTC/USD", kraken.TradeVolumeFee{Fee: decimal.NewFromFloat64(0.26)})
		price.SetReferenceCash(decimal.NewFromFloat64(200))
		price.SetQuote("BTC/USD", decimal.NewFromFloat64(59990), decimal.NewFromFloat64(60000))

		epoch := int64(300)
		writer := tables.NewWriter(catalog, epoch)
		now := time.Now().UTC()

		for tick := int64(1); tick <= 60; tick++ {
			for idx, pin := range testRegionPins {
				measurement := data.NewMeasurement(epoch, "BTC/USD", pin.source, tick, tick)
				measurement.At = now.Add(time.Duration(tick) * time.Second)
				measurement.From = measurement.At
				metric := data.NewMetric(pin.metric, 2.5+float64(idx), data.UnitCount, data.TimescaleTick)
				metric.Standardized = 2.5 + float64(idx)
				measurement.Write(metric)
				writer.Add(tables.Measurements, measurement)
			}
		}

		So(writer.CommitReady(ctx, true), ShouldBeNil)

		detections := make([]*data.Measurement, 0)

		for idx := int64(1); idx <= 6; idx++ {
			bTick := idx*6 + 4
			startTick := bTick - 3
			cTick := bTick + 2

			bPrice := decimal.NewFromFloat64(60000)
			cPrice := decimal.NewFromFloat64(63000)

			detection := data.NewMeasurement(
				epoch,
				"BTC/USD",
				"detector",
				startTick,
				cTick,
				&data.StringEntry{Key: "type", Value: "up"},
			)
			detection.At = now.Add(time.Duration(cTick) * time.Second)
			detection.From = now.Add(time.Duration(bTick) * time.Second)

			detection.Write(
				data.NewMetric("start_idx", float64(startTick), data.UnitCount, data.TimescaleTick),
				data.NewMetric("start_tick", float64(startTick), data.UnitCount, data.TimescaleTick),
				data.NewMetric("b_idx", float64(bTick), data.UnitCount, data.TimescaleTick),
				data.NewMetric("b_tick", float64(bTick), data.UnitCount, data.TimescaleTick),
				data.NewMetric("c_idx", float64(cTick), data.UnitCount, data.TimescaleTick),
				data.NewMetric("c_tick", float64(cTick), data.UnitCount, data.TimescaleTick),
				data.NewMetric("end_tick", float64(cTick), data.UnitCount, data.TimescaleTick),
				data.NewExactMetric("b_price", bPrice, data.UnitPrice, data.TimescaleTick),
				data.NewExactMetric("c_price", cPrice, data.UnitPrice, data.TimescaleTick),
			)
			detections = append(detections, detection)
		}

		// Also add a down excursion
		downBTick := int64(50)
		downStartTick := int64(46)
		downCTick := int64(52)
		downDetection := data.NewMeasurement(
			epoch,
			"BTC/USD",
			"detector",
			downStartTick,
			downCTick,
			&data.StringEntry{Key: "type", Value: "down"},
		)
		downDetection.At = now.Add(time.Duration(downCTick) * time.Second)
		downDetection.From = now.Add(time.Duration(downBTick) * time.Second)

		downDetection.Write(
			data.NewMetric("start_idx", float64(downStartTick), data.UnitCount, data.TimescaleTick),
			data.NewMetric("start_tick", float64(downStartTick), data.UnitCount, data.TimescaleTick),
			data.NewMetric("b_idx", float64(downBTick), data.UnitCount, data.TimescaleTick),
			data.NewMetric("b_tick", float64(downBTick), data.UnitCount, data.TimescaleTick),
			data.NewMetric("c_idx", float64(downCTick), data.UnitCount, data.TimescaleTick),
			data.NewMetric("c_tick", float64(downCTick), data.UnitCount, data.TimescaleTick),
			data.NewMetric("end_tick", float64(downCTick), data.UnitCount, data.TimescaleTick),
			data.NewExactMetric("b_price", decimal.NewFromFloat64(60000), data.UnitPrice, data.TimescaleTick),
			data.NewExactMetric("c_price", decimal.NewFromFloat64(58000), data.UnitPrice, data.TimescaleTick),
		)
		detections = append(detections, downDetection)

		So(catalog.Append(ctx, tables.Detections, epoch, detections), ShouldBeNil)

		trainer := NewTrainer(ctx, catalog, price)
		err := trainer.Generate(epoch)
		So(err, ShouldBeNil)
	})
}

func TestMCTSNode_UCB1Score(t *testing.T) {
	Convey("Given MCTS nodes evaluated under empirical reward normalization", t, func() {
		parent := newMCTSNode(1, 10, nil)
		parent.visits = 20

		childA := newMCTSNode(1, 5, parent)
		childA.visits = 10
		childA.totalPnL = 0.50 // mean reward = +0.05

		childB := newMCTSNode(6, 10, parent)
		childB.visits = 10
		childB.totalPnL = -0.50 // mean reward = -0.05

		parent.children = []*mctsNode{childA, childB}

		Convey("When rewards are normalized across empirical range [-0.05, 0.05]", func() {
			minReward := -0.05
			maxReward := 0.05
			explorationConstant := math.Sqrt(2)

			scoreA := childA.ucb1Score(explorationConstant, minReward, maxReward)
			scoreB := childB.ucb1Score(explorationConstant, minReward, maxReward)

			Convey("Exploitation properly prioritizes winning branches on unit scale", func() {
				// Child A has normalized exploitation = 1.0
				// Child B has normalized exploitation = 0.0
				// Visits are equal, so scoreA - scoreB == 1.0 exactly
				So(scoreA-scoreB, ShouldAlmostEqual, 1.0, 1e-12)
				So(scoreA, ShouldBeGreaterThan, scoreB)

				best := parent.selectChild(explorationConstant, minReward, maxReward)
				So(best, ShouldEqual, childA)
			})
		})
	})
}

