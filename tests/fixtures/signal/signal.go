package signal

import (
	"iter"
	"math"
	"time"
)

/*
State names the deterministic market regimes understood by every fixture.
*/
type State uint8

const (
	Baseline State = iota
	FastPump
	SlowPump
	FastDump
	SlowDump
)

const (
	idleObservations       = 16
	fastLegObservations    = 8
	slowLegObservations    = 16
	settleObservations     = 4
	initialPrice           = 100.0
	PriceIncrement         = 0.01
	idleAmplitudeFraction  = 0.0005
	eventMoveFraction      = 0.12
	idleVolume             = 10.0
	idleVolumeWaveFraction = 0.2
)

var epoch = time.Date(2026, 7, 21, 9, 0, 0, 0, time.UTC)

/*
Sample is one shared per-symbol market value written into every JSON template.
*/
type Sample struct {
	Symbol string
	Price  float64
	Volume float64
	At     time.Time
}

/*
Signal generates replayable per-symbol market tapes for the current transition.
*/
type Signal struct {
	symbols []string
	levels  map[string]float64
	at      time.Time
	phase   int
	state   State
	tape    [][]Sample
}

/*
New creates one deterministic signal shared by all fixtures in a market.
*/
func New(symbols []string) *Signal {
	levels := make(map[string]float64, len(symbols))

	for _, symbol := range symbols {
		levels[symbol] = initialPrice
	}

	return &Signal{
		symbols: append([]string(nil), symbols...),
		levels:  levels,
		at:      epoch,
	}
}

/*
Transition generates a finite event leg followed by idle samples at its result.
*/
func (signal *Signal) Transition(state State) {
	signal.state = state
	signal.tape = signal.tape[:0]
	leg := state.observations()
	total := leg

	if state != Baseline {
		total += settleObservations
	}

	start := make(map[string]float64, len(signal.levels))
	target := make(map[string]float64, len(signal.levels))

	for symbol, level := range signal.levels {
		start[symbol] = level
		target[symbol] = level * (1 + state.direction()*eventMoveFraction)

		if state == Baseline {
			target[symbol] = level
		}
	}

	for index := range total {
		interval := time.Second

		if index < leg {
			interval = state.interval()
		}

		signal.at = signal.at.Add(interval)
		step := make([]Sample, len(signal.symbols))

		for symbolIndex, symbol := range signal.symbols {
			center := target[symbol]

			if state != Baseline && index < leg {
				progress := float64(index+1) / float64(leg)
				progress = progress * progress * (3 - 2*progress)
				center = start[symbol] + (target[symbol]-start[symbol])*progress
			}

			wave := math.Sin(float64(signal.phase + index + symbolIndex))
			price := center * (1 + idleAmplitudeFraction*wave)
			volume := idleVolume * (1 + idleVolumeWaveFraction*math.Abs(wave))

			if state != Baseline && index < leg {
				volume = state.volume()
			}

			step[symbolIndex] = Sample{
				Symbol: symbol,
				Price:  math.Round(price/PriceIncrement) * PriceIncrement,
				Volume: volume,
				At:     signal.at,
			}
		}

		signal.tape = append(signal.tape, step)
	}

	for symbol, level := range target {
		signal.levels[symbol] = level
	}

	signal.phase += total
}

/*
Generate replays the current transition identically for every injected fixture.
*/
func (signal *Signal) Generate() iter.Seq[[]Sample] {
	return func(yield func([]Sample) bool) {
		for _, step := range signal.tape {
			if !yield(step) {
				return
			}
		}
	}
}

/*
State returns the semantic state represented by the current generated tape.
*/
func (signal *Signal) State() State {
	return signal.state
}

/*
observations returns the finite duration of the active event leg.
*/
func (state State) observations() int {
	switch state {
	case SlowPump, SlowDump:
		return slowLegObservations
	case FastPump, FastDump:
		return fastLegObservations
	default:
		return idleObservations
	}
}

/*
interval returns the sampling cadence of the active event leg.
*/
func (state State) interval() time.Duration {
	switch state {
	case FastPump, FastDump:
		return 250 * time.Millisecond
	default:
		return time.Second
	}
}

/*
direction returns the signed displacement of the selected event.
*/
func (state State) direction() float64 {
	switch state {
	case FastPump, SlowPump:
		return 1
	case FastDump, SlowDump:
		return -1
	default:
		return 0
	}
}

/*
volume returns the executed quantity generated during the active event leg.
*/
func (state State) volume() float64 {
	switch state {
	case FastPump, FastDump:
		return 100
	case SlowPump, SlowDump:
		return 30
	default:
		return idleVolume
	}
}
