package store

import (
	"context"
	"math"
	"slices"
	"sync"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
Grid is not a grid at all, it is a "group by" over a tick's signal layers
and Metric pins into 15 multi-signal confluence regions.
*/
type Grid struct {
	*runtime.System
	mu      sync.Mutex
	symbols map[string]*symbolState
}

/*
RegionCount is the number of confluence regions the pins map metrics into.
*/
const RegionCount = 15

func NewGrid(ctx context.Context) *Grid {
	return &Grid{
		System:  runtime.NewSystem(ctx, "grid"),
		symbols: make(map[string]*symbolState),
	}
}

/*
Region is one confluence region's evidence in a frame: Deformation is the sum
of its self-normalized metric magnitudes |SNM|, Counts how many metrics it holds,
and Brightness their average in [0, 1].
*/
type Region struct {
	Key         uint8
	Deformation float64
	Brightness  float64
	Counts      int
}

type metricKey struct {
	source string
	label  string
}

type prior struct {
	value    float64
	observed int
}

type symbolState struct {
	metrics  map[metricKey]prior
	energy   map[string]prior
	observed int
}

/*
SNM is the self-normalized movement of raw against prior.
*/
func SNM(raw, prior float64) float64 {
	delta := raw - prior
	denominator := math.Abs(prior) + math.Abs(delta)

	if denominator == 0 {
		return 0
	}

	return delta / denominator
}

/*
Observe reads the signal layers on value, advances each symbol by one tick,
and returns the 15 regions sorted brightest first. A metric's first tick,
and the tick after that (the source energy has no prior yet), score nothing.
A signal source or label with no pin is an error.
*/
func (grid *Grid) Observe(value *data.Value) ([]*Region, error) {
	if err := grid.Error(); err != nil {
		return nil, err
	}

	if value == nil {
		return nil, nil
	}

	type sample struct {
		key   metricKey
		value float64
	}

	bySymbol := make(map[string][]sample)

	for signal, err := range data.Signals(value) {
		if err != nil {
			return nil, err
		}

		pins, pinned := regionMetricPins[signal.Source]

		if !pinned {
			return nil, grid.Error(errnie.Err(
				errnie.Validation,
				"grid: "+signal.Symbol+" signal source "+signal.Source+" has no region pins",
				nil,
			))
		}

		for idx, val := range signal.Values {
			label := signal.Labels[idx]
			regions, pinned := pins[label]

			if !pinned {
				return nil, grid.Error(errnie.Err(
					errnie.Validation,
					"grid: "+signal.Symbol+" metric "+signal.Source+"/"+label+" is not pinned to a region",
					nil,
				))
			}

			if len(regions) == 0 {
				continue
			}

			bySymbol[signal.Symbol] = append(bySymbol[signal.Symbol], sample{
				key:   metricKey{signal.Source, label},
				value: val,
			})
		}
	}

	regions := make([]*Region, RegionCount)

	for idx := range regions {
		regions[idx] = &Region{Key: uint8(idx)}
	}

	grid.mu.Lock()
	defer grid.mu.Unlock()

	total := 0

	for symbol, samples := range bySymbol {
		state, ok := grid.symbols[symbol]

		if !ok {
			state = &symbolState{
				metrics: make(map[metricKey]prior),
				energy:  make(map[string]prior),
			}
			grid.symbols[symbol] = state
		}

		state.observed++
		now := state.observed
		movement := make(map[metricKey]float64)
		energy := make(map[string]float64)

		for _, sample := range samples {
			last, has := state.metrics[sample.key]
			state.metrics[sample.key] = prior{value: sample.value, observed: now}

			if !has || last.observed != now-1 {
				continue
			}

			delta := SNM(sample.value, last.value)
			movement[sample.key] = delta
			energy[sample.key.source] += delta * delta
		}

		surprise := make(map[string]float64)

		for source, amount := range energy {
			last, has := state.energy[source]
			state.energy[source] = prior{value: amount, observed: now}

			if has && last.observed == now-1 {
				surprise[source] = math.Abs(SNM(amount, last.value))
			}
		}

		for _, sample := range samples {
			delta, scored := movement[sample.key]
			sourceSurprise, surprised := surprise[sample.key.source]

			if !scored || !surprised {
				continue
			}

			for _, region := range regionMetricPins[sample.key.source][sample.key.label] {
				regions[region].Deformation += math.Abs(delta) * sourceSurprise
				regions[region].Counts++
				total++
			}
		}
	}

	if total == 0 {
		return nil, nil
	}

	for _, region := range regions {
		if region.Counts > 0 {
			region.Brightness = region.Deformation / float64(region.Counts)
		}
	}

	slices.SortFunc(regions, func(first, second *Region) int {
		if first.Brightness > second.Brightness {
			return -1
		}

		if first.Brightness < second.Brightness {
			return 1
		}

		return int(first.Key) - int(second.Key)
	})

	return regions, nil
}
