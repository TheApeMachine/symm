package signal

import (
	"fmt"
	"math"
	"time"
)

/*
Scenario expands a semantic helper into explicit book moves and touch trades.
When symbols are supplied, unselected symbols continue their idle tapes so one
instrument can move without fabricating a market-wide event.
*/
func (signal *Signal) Scenario(state State, symbols ...string) ([]Step, error) {
	if !state.valid() {
		return nil, fmt.Errorf("signal: unknown market state %d", state)
	}

	selected := make(map[string]bool, len(signal.symbols))

	if len(symbols) == 0 {
		for _, symbol := range signal.symbols {
			selected[symbol] = true
		}
	}

	for _, symbol := range symbols {
		if _, exists := signal.markets[symbol]; !exists {
			return nil, fmt.Errorf("signal: unknown symbol %q", symbol)
		}

		selected[symbol] = true
	}

	if state.isBookCondition() {
		return signal.bookCondition(state, selected), nil
	}

	leg := state.observations()
	total := leg

	if state != Baseline {
		total += settleObservations
	}

	levels := make(map[string]float64, len(signal.markets))
	targets := make(map[string]float64, len(signal.markets))

	for symbol, market := range signal.markets {
		level := market.mid()
		levels[symbol] = level
		targets[symbol] = level * (1 + state.direction()*eventMoveFraction)

		if state == Baseline || !selected[symbol] {
			targets[symbol] = level
		}
	}

	steps := make([]Step, total)

	for index := range total {
		advance := time.Second

		if index < leg {
			advance = state.interval()
		}

		actions := make([]Action, 0, len(signal.symbols)*2)

		for symbolIndex, symbol := range signal.symbols {
			active := state != Baseline && selected[symbol] && index < leg
			center := targets[symbol]

			if active {
				progress := float64(index+1) / float64(leg)
				progress = progress * progress * (3 - 2*progress)
				mid := signal.markets[symbol].mid()
				center = mid + (targets[symbol]-mid)*progress
			}

			wave := math.Sin(float64(signal.phase + index + symbolIndex))
			desired := center * (1 + idleAmplitudeFraction*wave)
			desired = math.Round(desired/PriceIncrement) * PriceIncrement
			ticks := int64(math.Round((desired - levels[symbol]) / PriceIncrement))
			levels[symbol] += float64(ticks) * PriceIncrement
			side := "buy"

			if state == FastDump || state == SlowDump ||
				(!active || state == SpreadCompression) &&
					(index+symbolIndex)%2 != 0 {
				side = "sell"
			}

			volume := idleVolume * (1 + idleVolumeWaveFraction*math.Abs(wave))

			if active {
				volume = state.volume()
			}

			actions = append(actions, Action{
				Kind: Trade, Symbol: symbol, Side: side, Qty: volume,
			})
			actions = append(actions, Action{
				Kind: MoveMid, Symbol: symbol, Ticks: ticks,
			})

			compressing := state == SpreadCompression && active

			if compressing && index == 0 {
				actions = append(actions, Action{
					Kind: TightenSpread, Symbol: symbol, Ticks: 1,
				})
			}

			if !compressing {
				switch index % spreadCycleLength {
				case spreadWidenPhase:
					actions = append(actions, Action{
						Kind: WidenSpread, Symbol: symbol, Ticks: 1,
					})
				case spreadTightenPhase:
					actions = append(actions, Action{
						Kind: TightenSpread, Symbol: symbol, Ticks: 1,
					})
				}
			}
		}

		steps[index] = Step{Advance: advance, Actions: actions}
	}

	return steps, nil
}
