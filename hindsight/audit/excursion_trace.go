package audit

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/apache/iceberg-go"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/strategy"
)

/*
ExtractExcursionTraces extracts detailed multi-region activation transitions for
informative detected excursions (prioritizing profitable long ignition and holding),
deduplicating consecutive ticks with identical winning region tokens.
*/
func ExtractExcursionTraces(
	ctx context.Context,
	catalog *tables.Catalog,
	epoch int64,
	grid *store.Grid,
	detections []*data.Measurement,
	tickMeasurements map[int64][]*data.Measurement,
	maxTraces int,
) []ExcursionTransitionTrace {
	if grid == nil || len(detections) == 0 {
		return nil
	}

	limit := maxTraces
	if limit <= 0 {
		limit = 5
	}

	type candidate struct {
		det       *data.Measurement
		returnPct float64
	}

	candidates := make([]candidate, 0, len(detections))

	for _, det := range detections {
		if det == nil {
			continue
		}

		typeEntry := data.Pull(det.Meta("type"))

		if typeEntry == nil || typeEntry.Value == "" {
			continue
		}

		bPrice := data.Pull(det.Read("b_price")).Metric.Raw()
		cPrice := data.Pull(det.Read("c_price")).Metric.Raw()

		returnPct := 0.0
		if bPrice > 0 && cPrice > 0 {
			returnPct = ((cPrice - bPrice) / bPrice) * 100.0
		}

		// Prioritize up excursions, but keep others if up are scarce
		candidates = append(candidates, candidate{det: det, returnPct: returnPct})
	}

	slices.SortFunc(candidates, func(left, right candidate) int {
		leftType := data.Pull(left.det.Meta("type"))
		rightType := data.Pull(right.det.Meta("type"))
		leftUp := leftType != nil && leftType.Value == "up"
		rightUp := rightType != nil && rightType.Value == "up"

		if leftUp && !rightUp {
			return -1
		}

		if !leftUp && rightUp {
			return 1
		}

		if left.returnPct > right.returnPct {
			return -1
		}

		if left.returnPct < right.returnPct {
			return 1
		}

		return 0
	})

	traces := make([]ExcursionTransitionTrace, 0, limit)

	for _, cand := range candidates {
		if len(traces) >= limit {
			break
		}

		det := cand.det
		symbol := det.Label
		var class string

		if typeEntry := data.Pull(det.Meta("type")); typeEntry != nil {
			class = typeEntry.Value
		}

		startTick := int64(data.Pull(det.Read("start_tick")).Metric.Raw())
		bTick := int64(data.Pull(det.Read("b_tick")).Metric.Raw())
		cTick := int64(data.Pull(det.Read("c_tick")).Metric.Raw())

		if bTick <= 0 || cTick <= bTick {
			continue
		}

		lo, _ := strategy.PadWindow(bTick, cTick)
		if startTick > 0 && startTick < lo {
			lo = startTick
		}
		startTick = lo

		if bTick <= startTick {
			continue
		}

		bPrice := data.Pull(det.Read("b_price")).Metric.Raw()
		cPrice := data.Pull(det.Read("c_price")).Metric.Raw()

		// Gather tick signals across [startTick, cTick]
		ticksMap := make(map[int64][]*data.Measurement)
		hasSample := false

		for tick := startTick; tick <= cTick; tick++ {
			list, exists := tickMeasurements[tick]
			if !exists {
				continue
			}

			for _, measurement := range list {
				if measurement != nil && (symbol == "" || measurement.Label == symbol) {
					ticksMap[tick] = append(ticksMap[tick], measurement)
					hasSample = true
				}
			}
		}

		if !hasSample && catalog != nil && epoch > 0 {
			for meas, tapeErr := range catalog.SignalLogic(
				ctx, epoch, symbol, startTick, cTick, tables.SensorySources...,
			) {
				if tapeErr != nil {
					errnie.Error(tapeErr)
					break
				}

				if meas != nil && (symbol == "" || meas.Label == symbol) {
					ticksMap[meas.Tick] = append(ticksMap[meas.Tick], meas)
				}
			}
		}

		if len(ticksMap) == 0 {
			continue
		}

		tradePrices := make(map[int64]float64)

		if catalog != nil && epoch > 0 {
			filter := iceberg.NewAnd(
				iceberg.NewAnd(
					iceberg.EqualTo(iceberg.Reference("source"), "spot:trade"),
					iceberg.EqualTo(iceberg.Reference("label"), symbol),
				),
				iceberg.NewAnd(
					iceberg.GreaterThanEqual(iceberg.Reference("tick"), startTick),
					iceberg.LessThanEqual(iceberg.Reference("tick"), cTick),
				),
			)

			for trade, err := range catalog.Scan(ctx, tables.Measurements, epoch, filter, 0) {
				if err != nil {
					errnie.Error(errnie.Err(
						errnie.IO,
						"[audit.excursion_trace] failed to read trade stream for excursion",
						err,
					), "symbol", symbol, "startTick", startTick, "cTick", cTick)

					break
				}

				if trade == nil {
					continue
				}

				if priceEntry := data.Pull(trade.Read("price")); priceEntry != nil && priceEntry.Metric != nil {
					tradePrices[trade.Tick] = priceEntry.Metric.Raw()
				}
			}
		}

		if len(tradePrices) == 0 {
			errnie.Error(errnie.Err(
				errnie.NotFound,
				"[audit.excursion_trace] zero trades found for excursion interval",
				nil,
			), "symbol", symbol, "startTick", startTick, "cTick", cTick)

			continue
		}

		var prevailingPrice float64
		var earliestTradeTick int64 = math.MaxInt64

		for tradeTick, tradePrice := range tradePrices {
			if tradeTick < earliestTradeTick && tradePrice > 0 {
				earliestTradeTick = tradeTick
				prevailingPrice = tradePrice
			}
		}

		if prevailingPrice <= 0 || bPrice <= 0 {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"[audit.excursion_trace] invalid baseline or prevailing price for excursion",
				nil,
			), "symbol", symbol, "prevailingPrice", prevailingPrice, "bPrice", bPrice)

			continue
		}

		sortedTicks := make([]int64, 0, len(ticksMap))
		for tick := range ticksMap {
			sortedTicks = append(sortedTicks, tick)
		}
		slices.Sort(sortedTicks)

		frames := make([]ExcursionTransitionFrame, 0)
		prevToken := ""
		dwellCount := 0

		for _, tick := range sortedTicks {
			signals := ticksMap[tick]
			if len(signals) == 0 {
				continue
			}

			frame := data.NewMeasurement(
				signals[0].Epoch,
				symbol,
				"trace",
				signals[0].At,
				signals[0].From,
			)
			frame.SeqIdx = signals[0].SeqIdx
			frame.Tick = tick
			frame.Peers(signals...)
			frame.Write()

			regions := grid.Observe(frame)

			if regions == nil {
				continue
			}

			token := fmt.Sprintf("R%02d", regions[0].Key)
			top3Token, _ := (&strategy.Tokenizer{Regions: 3}).Token(regions)
			var brightness [store.RegionCount]float64
			var counts [store.RegionCount]int

			for _, region := range regions {
				brightness[region.Key] = region.Brightness
				counts[region.Key] = region.Counts
			}

			if currentPrice, hasPrice := tradePrices[tick]; hasPrice && currentPrice > 0 {
				prevailingPrice = currentPrice
			}

			tickReturnPct := ((prevailingPrice - bPrice) / bPrice) * 100.0

			phase := "precursor"
			if tick >= bTick && tick <= cTick {
				phase = "excursion"
			}

			if tick > cTick {
				phase = "exhaustion"
			}

			// Deduplicate consecutive ticks with the same winner token
			if len(frames) == 0 || token != prevToken {
				if len(frames) > 0 {
					frames[len(frames)-1].DwellTicks = dwellCount
				}

				prevToken = token
				dwellCount = 1

				transitionFrame := ExcursionTransitionFrame{
					Step:       len(frames),
					Tick:       tick,
					Phase:      phase,
					Token:      token,
					Top3Token:  top3Token,
					Winner:     regions[0].Key,
					RunnerUp:   regions[1].Key,
					Brightness: brightness,
					Counts:     counts,
					DwellTicks: 1,
					Price:      prevailingPrice,
					ReturnPct:  tickReturnPct,
				}
				frames = append(frames, transitionFrame)
				continue
			}

			dwellCount++
		}

		if len(frames) > 0 {
			frames[len(frames)-1].DwellTicks = dwellCount
		}

		if len(frames) < 3 {
			continue
		}

		traceReturn := cand.returnPct
		traces = append(traces, ExcursionTransitionTrace{
			Symbol:      symbol,
			Class:       class,
			StartTick:   startTick,
			BTick:       bTick,
			CTick:       cTick,
			BPrice:      bPrice,
			CPrice:      cPrice,
			ReturnPct:   traceReturn,
			Transitions: frames,
		})
	}

	return traces
}

/*
FormatTransitionSequence returns a human-readable arrow-delimited sequence of transitions.
*/
func FormatTransitionSequence(trace ExcursionTransitionTrace) string {
	parts := make([]string, 0, len(trace.Transitions))
	for _, transition := range trace.Transitions {
		parts = append(parts, fmt.Sprintf("%s(%d)", transition.Token, transition.DwellTicks))
	}
	return strings.Join(parts, " -> ")
}
