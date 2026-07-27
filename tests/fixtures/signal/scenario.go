package signal

import (
	"fmt"
	"math"
	"time"
)

func progressFor(index, start, observations int) float64 {
	progress := float64(index-start+1) / float64(observations)
	return progress * progress * (3 - 2*progress)
}

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
		steps := signal.bookCondition(state, selected)
		draft := signal.clone()

		for _, step := range steps {
			if err := draft.Apply(step); err != nil {
				return nil, err
			}
		}

		return steps, nil
	}

	leg := state.observations()

	if state == LeaderFollower {
		leg += (len(signal.symbols) - 1) * leaderLagObservations
	}

	total := leg

	if state != Baseline {
		total += settleObservations
	}

	levels := make(map[string]float64, len(signal.markets))
	targets := make(map[string]float64, len(signal.markets))

	for symbolIndex, symbol := range signal.symbols {
		market := signal.markets[symbol]
		level := market.mid()
		levels[symbol] = level
		moveFraction := state.direction() * eventMoveFraction

		if state == SmallDisplacementLift {
			moveFraction = smallMoveFraction
		}

		if state == BullTrend || state == BearTrend {
			moveFraction = state.direction() * (eventMoveFraction * 1.5)
		}

		if state == LeaderFollower {
			moveFraction = eventMoveFraction / float64(symbolIndex+1)
		}

		if state == AdverseDivergence && symbolIndex == 0 {
			moveFraction = -eventMoveFraction
		}

		targets[symbol] = level * (1 + moveFraction)

		if state == Baseline || !selected[symbol] {
			targets[symbol] = level
		}
	}

	steps := make([]Step, total)
	draft := signal.clone()

	for index := range total {
		advance := time.Second

		if index < leg {
			advance = state.interval()
		}

		actions := make([]Action, 0, len(signal.symbols)*2)

		for symbolIndex, symbol := range signal.symbols {
			start := 0
			activeObservations := leg

			if state == LeaderFollower {
				start = symbolIndex * leaderLagObservations
				activeObservations = fastLegObservations
			}

			active := state != Baseline && selected[symbol] &&
				index >= start && index < start+activeObservations

			if state == LeaderFollower {
				leaderLag := activeObservations / 4

				switch symbolIndex {
				case 1:
					active = active && index >= start+leaderLag
				case 2:
					active = active && index >= start+(2*leaderLag)
				}
			}

			center := targets[symbol]

			if state == LeaderFollower && index < start {
				center = signal.markets[symbol].mid()
			}

			if active {
				progress := float64(index-start+1) / float64(activeObservations)
				progress = progress * progress * (3 - 2*progress)
				mid := signal.markets[symbol].mid()
				center = mid + (targets[symbol]-mid)*progress

				switch state {
				case SidewaysChop:
					center = levels[symbol] * (1 + (eventMoveFraction/10)*math.Sin(progress*6*math.Pi))
				case VolatilitySpike:
					center = levels[symbol] * (1 + (eventMoveFraction/2)*math.Sin(progress*8*math.Pi))
				case SuddenReversal:
					if progress <= 0.5 {
						center = levels[symbol] * (1 + eventMoveFraction*(progress*2))
					} else {
						center = levels[symbol] * (1 + eventMoveFraction - (eventMoveFraction * 2.2 * (progress - 0.5) * 2))
					}
				case FlashCrash:
					if progress <= 0.5 {
						center = levels[symbol] * (1 - 0.35*(progress*2))
					} else {
						center = levels[symbol] * (0.65 + 0.27*((progress-0.5)*2))
					}
				}
			}

			wave := math.Sin(float64(signal.phase + index + symbolIndex))
			desired := center * (1 + idleAmplitudeFraction*wave)

			if (state == VolumeAbsorption || state == SpreadCompression ||
				state == SpreadControl) && selected[symbol] {
				desired = levels[symbol]
			}

			desired = math.Round(desired/PriceIncrement) * PriceIncrement
			ticks := int64(math.Round((desired - levels[symbol]) / PriceIncrement))
			levels[symbol] += float64(ticks) * PriceIncrement
			side := "buy"

			if active && (state == FastDump || state == SlowDump ||
				state == AdverseDivergence && symbolIndex == 0 ||
				state == BearTrend) {
				side = "sell"
			}

			if active && state == SidewaysChop {
				if (index+symbolIndex)%2 != 0 {
					side = "sell"
				}
			}

			if active && state == VolatilitySpike {
				if math.Sin(float64(index+symbolIndex)) < 0 {
					side = "sell"
				}
			}

			if active && state == SuddenReversal && progressFor(index, start, activeObservations) > 0.5 {
				side = "sell"
			}

			if active && state == FlashCrash {
				if progressFor(index, start, activeObservations) <= 0.5 {
					side = "sell"
				}
			}

			if (!active || state == SpreadCompression) &&
				(index+symbolIndex)%2 != 0 {
				side = "sell"
			}

			volume := round(idleVolume * (1 + idleVolumeWaveFraction*math.Abs(wave)))

			if active {
				volume = state.volume()

				if state == VolatilitySpike {
					volume = 180
				}

				if state == FlashCrash && progressFor(index, start, activeObservations) <= 0.5 {
					volume = 220
				}
			}

			actions = append(actions, Action{
				Kind: Trade, Symbol: symbol, Side: side, Qty: volume,
			})
			liquiditySide := "sell"

			if side == "sell" {
				liquiditySide = "buy"
			}

			actions = append(actions, Action{
				Kind: Refill, Symbol: symbol, Side: liquiditySide, Qty: volume,
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

			absorbing := state == VolumeAbsorption && selected[symbol]

			if !compressing && !absorbing {
				switch index % spreadCycleLength {
				case spreadWidenPhase:
					widen := int64(1)

					if state == VolatilitySpike || state == FlashCrash {
						widen = 3
					}

					actions = append(actions, Action{
						Kind: WidenSpread, Symbol: symbol, Ticks: widen,
					})
				case spreadTightenPhase:
					actions = append(actions, Action{
						Kind: TightenSpread, Symbol: symbol, Ticks: 1,
					})
				}
			}
		}

		steps[index] = Step{Advance: advance, Actions: actions}

		if err := draft.Apply(steps[index]); err != nil {
			return nil, err
		}
	}

	return steps, nil
}
