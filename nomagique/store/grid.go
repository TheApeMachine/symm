package store

import (
	"context"
	"math"
	"slices"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/nomagique/runtime"
)

type Prior struct {
	Signal     broker.Sample
	Samples    int64
	Prediction float64
	Energy     float64
}

/*
Grid is not a grid at all, it is a "group by" over Measurement
and Metric pins into 15 multi-signal confluence regions.
*/
type Grid struct {
	*runtime.System
	Priors map[string]map[string]*Prior
}

/*
RegionCount is the number of confluence regions the pins map metrics into.
*/
const RegionCount = 15

func NewGrid(ctx context.Context) *Grid {
	return &Grid{
		System: runtime.NewSystem(ctx, "grid"),
		Priors: make(map[string]map[string]*Prior),
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

/*
Observe applies every signal layer for symbol on this tick, then returns the
lit regions at or above the brightness midpoint.
*/
func (grid *Grid) Observe(symbol string, signals []broker.Sample) ([]*Region, error) {
	if symbol == "" {
		return nil, nil
	}

	if err := grid.Error(); err != nil {
		return nil, err
	}

	var byKey [RegionCount]*Region
	regions := make([]*Region, 0, RegionCount)

	for _, signal := range signals {
		if signal.Symbol != symbol {
			continue
		}

		if err := pinned(symbol, signal); err != nil {
			return nil, err
		}

		if grid.Priors[signal.Source] == nil {
			grid.Priors[signal.Source] = make(map[string]*Prior)
		}

		prior := grid.Priors[signal.Source][symbol]

		if prior == nil {
			grid.Priors[signal.Source][symbol] = &Prior{Signal: signal}
			continue
		}

		if prior.Samples > 0 {
			prior.Prediction += (prior.Energy - prior.Prediction) / float64(prior.Samples)
		}

		energy := 0.0

		for idx := range signal.Values {
			snm := grid.SNM(&signal, idx)
			energy += snm * snm
		}

		excitation := 1 + grid.Surprise(prior.Prediction, energy)
		sourcePins, ok := regionMetricPins[signal.Source]

		if !ok {
			prior.Signal = signal
			prior.Energy = energy
			prior.Samples++
			continue
		}

		for idx, label := range signal.Labels {
			targets := sourcePins[label]

			if len(targets) == 0 {
				continue
			}

			for _, key := range targets {
				if key >= RegionCount {
					continue
				}

				region := byKey[key]

				if region == nil {
					region = &Region{Key: key}
					byKey[key] = region
					regions = append(regions, region)
				}

				region.Deformation += math.Abs(grid.SNM(&signal, idx)) * excitation
				region.Counts++
				region.Brightness = region.Deformation / float64(region.Counts)
			}
		}

		prior.Signal = signal
		prior.Energy = energy
		prior.Samples++
	}

	return grid.finalizeRegions(regions), nil
}

/*
pinned rejects a label that the source map does not name. An unknown source
has no pins to check. The error is returned to the caller and is not stored
on the grid.
*/
func pinned(symbol string, signal broker.Sample) error {
	sourcePins, ok := regionMetricPins[signal.Source]

	if !ok {
		return nil
	}

	for _, label := range signal.Labels {
		if _, ok := sourcePins[label]; ok {
			continue
		}

		return errnie.Err(
			errnie.Validation,
			"grid: "+symbol+" metric "+signal.Source+"/"+label+" is not pinned to a region",
			nil,
		)
	}

	return nil
}

func (grid *Grid) finalizeRegions(regions []*Region) []*Region {
	if len(regions) == 0 {
		return nil
	}

	slices.SortFunc(regions, func(a, b *Region) int {
		if a.Brightness > b.Brightness {
			return -1
		}

		if a.Brightness < b.Brightness {
			return 1
		}

		return int(a.Key) - int(b.Key)
	})

	midB := (regions[0].Brightness + regions[len(regions)-1].Brightness) / 2

	cut, _ := slices.BinarySearchFunc(regions, midB, func(r *Region, mid float64) int {
		if r.Brightness >= mid {
			return -1
		}

		return 1
	})

	return regions[:cut]
}

/*
SNM (Self-Normalized Movement) takes the reference from the prior magnitude
and the movement itself. It measures a transition, not an isolated value.
The result stays within \([-1,1]\) without statistical calibration.
*/
func (grid *Grid) SNM(signal *broker.Sample, idx int) float64 {
	prior := grid.Priors[signal.Source][signal.Symbol]

	if prior == nil || idx >= len(signal.Values) || idx >= len(prior.Signal.Values) {
		return 0
	}

	rawPrior := prior.Signal.Values[idx]
	delta := signal.Values[idx] - rawPrior
	denominator := math.Abs(rawPrior) + math.Abs(delta)

	if denominator == 0 {
		return 0
	}

	return delta / denominator
}

/*
Surprise returns the MDL prediction error: how much the current
total metric energy deviates from the expected regime baseline.
*/
func (grid *Grid) Surprise(prediction, energy float64) float64 {
	if prediction <= 0 {
		return energy
	}

	return math.Abs(energy - prediction)
}

/*
Maturity calculates sensor readiness based strictly on
empirical sample history and metric completeness (warm-up gate).
*/
func (grid *Grid) Maturity(signal *broker.Sample) float64 {
	prior := grid.Priors[signal.Source][signal.Symbol]

	if prior == nil || prior.Samples <= 0 {
		return 0
	}

	samplesCount := float64(prior.Samples)

	maturity := 1.0 - (1.0 / samplesCount)

	if expected := float64(len(regionMetricPins[prior.Signal.Source])); expected > 0 {
		actual := float64(len(prior.Signal.Labels))

		if actual < expected {
			maturity *= (actual / expected)
		}
	}

	return maturity
}
