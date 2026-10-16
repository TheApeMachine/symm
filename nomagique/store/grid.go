package store

import (
	"context"
	"fmt"
	"math"
	"slices"
	"sync"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
Grid groups one symbol's signal layers of a tick into the 15 confluence
regions regionMetricPins declares, on self-normalized movement.

Stored ticks carry raw signal values only. Normalization happens here, at
read time, identically live and in replay, because it depends only on the
order of the ticks a Grid observes:

  - SNM of a metric is (raw - prior) / (|prior| + |raw - prior|), where prior
    is the same <symbol>/<source>/<label> in the previous tick the Grid
    observed for that symbol. It is bounded in [-1, 1]; raw == prior is 0.
  - A metric with no prior (first tick of the symbol, or absent from the
    previous tick: a gap) is warming up: it has no SNM and is counted in
    Frame.Warming, never scored as 0.
  - Energy of a source is the sum of SNM^2 of its metrics on this tick. Its
    prediction is the source's previous energy; surprise is the magnitude of
    the SNM of energy against that prediction, in [0, 1]. A source without a
    previous energy is warming up and contributes nothing.
  - A region's Deformation is the sum of |SNM| * surprise of its pinned
    metrics, Brightness their mean.

A key that appears in several layers of one tick (signals that write once per
trade) takes its last value: the tick's final state.
*/
type Grid struct {
	*runtime.System
	mu      sync.Mutex
	symbols map[string]*symbolState
}

type symbolState struct {
	observed uint64
	metrics  map[metricKey]prior
	energy   map[string]prior
}

type metricKey struct{ source, label string }

type prior struct {
	value    float64
	observed uint64
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
of |SNM| * surprise of its metrics, Counts how many metrics it holds, and
Brightness their average.
*/
type Region struct {
	Key         uint8
	Deformation float64
	Brightness  float64
	Counts      int
}

/*
Frame is one Observe: Regions sorted brightest first (nil when no pinned
metric was scorable), and Warming the pinned metrics still warming up.
*/
type Frame struct {
	Regions []*Region
	Warming int
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
Observe advances symbol by one tick on value's signal layers and returns its
frame. A signal layer that does not parse, or carries a source or label
regionMetricPins does not pin, is an error: every key a signal writes must
have a declared place in the grid.
*/
func (grid *Grid) Observe(value *data.Value, symbol string) (*Frame, error) {
	if err := grid.Error(); err != nil {
		return nil, err
	}

	raw := make(map[metricKey]float64)
	order := make([]metricKey, 0)

	for signal, err := range data.Signals(value) {
		if err != nil {
			return nil, err
		}

		if signal.Symbol != symbol {
			continue
		}

		sourcePins, pinned := regionMetricPins[signal.Source]

		if !pinned {
			return nil, errnie.Err(errnie.Validation, fmt.Sprintf(
				"grid: %s signal source %s has no region pins", symbol, signal.Source,
			), nil)
		}

		for idx, val := range signal.Values {
			key := metricKey{signal.Source, signal.Labels[idx]}

			if _, pinned := sourcePins[key.label]; !pinned {
				return nil, errnie.Err(errnie.Validation, fmt.Sprintf(
					"grid: %s metric %s/%s is not pinned to a region", symbol, key.source, key.label,
				), nil)
			}

			if _, seen := raw[key]; !seen {
				order = append(order, key)
			}

			raw[key] = val
		}
	}

	grid.mu.Lock()
	defer grid.mu.Unlock()

	state, ok := grid.symbols[symbol]

	if !ok {
		state = &symbolState{metrics: make(map[metricKey]prior), energy: make(map[string]prior)}
		grid.symbols[symbol] = state
	}

	state.observed++
	now := state.observed

	snm := make(map[metricKey]float64, len(raw))
	energy := make(map[string]float64)
	sources := make([]string, 0)

	for _, key := range order {
		last, has := state.metrics[key]
		state.metrics[key] = prior{value: raw[key], observed: now}

		if !has || last.observed != now-1 {
			continue
		}

		movement := SNM(raw[key], last.value)
		snm[key] = movement

		if _, seen := energy[key.source]; !seen {
			sources = append(sources, key.source)
		}

		energy[key.source] += movement * movement
	}

	surprise := make(map[string]float64, len(sources))

	for _, source := range sources {
		last, has := state.energy[source]
		state.energy[source] = prior{value: energy[source], observed: now}

		if has && last.observed == now-1 {
			surprise[source] = math.Abs(SNM(energy[source], last.value))
		}
	}

	frame := &Frame{}
	regions := make([]*Region, RegionCount)

	for idx := range regions {
		regions[idx] = &Region{Key: uint8(idx)}
	}

	totalCounts := 0

	for _, key := range order {
		targetRegions := regionMetricPins[key.source][key.label]

		movement, scored := snm[key]
		sourceSurprise, surprised := surprise[key.source]

		if !scored || !surprised {
			frame.Warming++
			continue
		}

		for _, region := range targetRegions {
			regions[region].Deformation += math.Abs(movement) * sourceSurprise
			regions[region].Counts++
			totalCounts++
		}
	}

	if totalCounts == 0 {
		return frame, nil
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

	frame.Regions = regions
	return frame, nil
}
