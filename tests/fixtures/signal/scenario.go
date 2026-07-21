package signal

import (
	"fmt"
	"math"
	"time"
)

/*
Scenario expands a semantic helper into explicit book moves and touch trades.
*/
func (signal *Signal) Scenario(state State) ([]Step, error) {
	if !state.valid() {
		return nil, fmt.Errorf("signal: unknown market state %d", state)
	}

	leg := state.observations()
	total := leg

	if state != Baseline {
		total += settleObservations
	}

	levels := make(map[string]float64, len(signal.markets))
	targets := make(map[string]float64, len(signal.markets))

	for symbol, market := range signal.markets {
		level := market.mid
		levels[symbol] = level
		targets[symbol] = level * (1 + state.direction()*eventMoveFraction)

		if state == Baseline {
			targets[symbol] = level
		}
	}

	steps := make([]Step, total)

	for index := range total {
		active := state != Baseline && index < leg
		advance := time.Second

		if index < leg {
			advance = state.interval()
		}

		actions := make([]Action, 0, len(signal.symbols)*2)

		for symbolIndex, symbol := range signal.symbols {
			center := targets[symbol]

			if active {
				progress := float64(index+1) / float64(leg)
				progress = progress * progress * (3 - 2*progress)
				center = signal.markets[symbol].mid +
					(targets[symbol]-signal.markets[symbol].mid)*progress
			}

			wave := math.Sin(float64(signal.phase + index + symbolIndex))
			desired := center * (1 + idleAmplitudeFraction*wave)
			desired = math.Round(desired/PriceIncrement) * PriceIncrement
			ticks := int64(math.Round((desired - levels[symbol]) / PriceIncrement))
			levels[symbol] += float64(ticks) * PriceIncrement
			side := "buy"

			if state == FastDump || state == SlowDump || state == Baseline && (index+symbolIndex)%2 != 0 {
				side = "sell"
			}

			volume := idleVolume * (1 + idleVolumeWaveFraction*math.Abs(wave))

			if active {
				volume = state.volume()
			}

			actions = append(actions,
				Action{Kind: MoveMid, Symbol: symbol, Ticks: ticks},
				Action{Kind: Trade, Symbol: symbol, Side: side, Qty: volume},
			)
		}

		steps[index] = Step{Advance: advance, Actions: actions}
	}

	signal.state = state
	signal.phase += total
	return steps, nil
}
