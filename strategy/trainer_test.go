package strategy

import (
	"context"
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

func TestTrainer_MCTSPrecursorWindowOptimization(t *testing.T) {
	Convey("1. Different precursor tick counts produce naturally varying token depths", t, func() {
		grid := store.NewGrid()
		tapeByTick := make(map[int64][]*data.Measurement)
		epoch := int64(100)
		symbol := "BTC/USD"
		now := time.Now().UTC()

		for tick := int64(1); tick <= 30; tick++ {
			meas := data.NewMeasurement(epoch, symbol, "hawkes", tick, tick)
			meas.At = now.Add(time.Duration(tick) * time.Second)
			meas.From = meas.At

			// Alter metrics every 3 ticks to cause natural region transitions
			standardVal := float64((tick / 3) + 1)
			metric := data.NewMetric("intensity:buy", standardVal, data.UnitCount, data.TimescaleTick)
			metric.Standardized = standardVal
			meas.Write(metric)
			tapeByTick[tick] = []*data.Measurement{meas}
		}

		bTick := int64(25)

		// Short window of 4 ticks (ticks 21..24)
		shortWindow := int64(4)
		shortTokens := tokenizeTape(grid, tapeByTick, bTick-shortWindow, bTick, epoch, symbol, 0, false)

		// Longer window of 18 ticks (ticks 7..24)
		longWindow := int64(18)
		longTokens := tokenizeTape(grid, tapeByTick, bTick-longWindow, bTick, epoch, symbol, 0, false)

		So(len(shortTokens), ShouldBeGreaterThan, 0)
		So(len(longTokens), ShouldBeGreaterThan, len(shortTokens))
	})

	Convey("2. Consecutive identical region tokens are collapsed", t, func() {
		grid := store.NewGrid()
		tapeByTick := make(map[int64][]*data.Measurement)
		epoch := int64(100)
		symbol := "BTC/USD"
		now := time.Now().UTC()

		// 10 identical ticks producing the same token
		for tick := int64(1); tick <= 10; tick++ {
			meas := data.NewMeasurement(epoch, symbol, "hawkes", tick, tick)
			meas.At = now.Add(time.Duration(tick) * time.Second)
			meas.From = meas.At
			metric := data.NewMetric("intensity:buy", 2.0, data.UnitCount, data.TimescaleTick)
			metric.Standardized = 2.0
			meas.Write(metric)
			tapeByTick[tick] = []*data.Measurement{meas}
		}

		tokens := tokenizeTape(grid, tapeByTick, 1, 11, epoch, symbol, 0, false)
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
			meas := data.NewMeasurement(epoch, symbol, "hawkes", tick, tick)
			meas.At = now.Add(time.Duration(tick) * time.Second)
			meas.From = meas.At
			metric := data.NewMetric("intensity:buy", float64(tick%5), data.UnitCount, data.TimescaleTick)
			metric.Standardized = float64(tick % 5)
			meas.Write(metric)
			tapeByTick[tick] = []*data.Measurement{meas}
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
			measurement := data.NewMeasurement(epoch, "BTC/USD", "hawkes", tick, tick)
			measurement.At = now.Add(time.Duration(tick) * time.Second)
			measurement.From = measurement.At
			metric := data.NewMetric("intensity:buy", 2.5, data.UnitCount, data.TimescaleTick)
			metric.Standardized = 2.5
			measurement.Write(metric)
			writer.Add(tables.Measurements, measurement)
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
