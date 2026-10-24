package store

import (
	"context"
	"math"
	"slices"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

type Prior struct {
	Signal     Signal
	Samples    int64
	Prediction float64
	Energy     float64
}

/*
Grid maps pinned metrics for one symbol into 15 confluence regions and
maintains per-source priors for SNM and energy surprise.
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
Region is one confluence region's evidence in a frame. Deformation is the
sum of |SNM| scaled by excitation (one plus that source's energy surprise).
Counts is how many pinned metrics landed here. Brightness is that average,
deformation divided by counts. Excitation can push it above 1. The JSON
tags are the dashboard contract: the Impulse Map renders the grid's own
accepted regions from the training frame's grid/regions layer.
*/
type Region struct {
	Symbol      string  `json:"symbol"`
	Key         uint8   `json:"key"`
	Deformation float64 `json:"deformation"`
	Brightness  float64 `json:"brightness"`
	Counts      int     `json:"counts"`
}

type Signal struct {
	Source string
	Symbol string
	Labels []string
	Values []float64
}

/*
Observe applies this tick's metrics for one pair, updates priors, and returns
the regions those metrics landed in.
*/
func (grid *Grid) Observe(
	symbol string,
	sources map[string]map[string]float64,
	enter bool,
) ([]*Region, error) {
	if symbol == "" {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"[grid.Observe] empty symbol",
			nil,
		))
	}

	if err := grid.Error(); err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"[grid.Observe] grid is in error state",
			err,
		))
	}

	if len(sources) == 0 {
		return nil, nil
	}

	var slots [RegionCount]*Region
	regions := make([]*Region, 0, RegionCount)

	for source, labels := range sources {
		if len(labels) == 0 {
			continue
		}

		if err := grid.ingestSignal(&slots, &regions, source, symbol, labels); err != nil {
			return nil, err
		}
	}

	for _, region := range regions {
		if region == nil || region.Counts == 0 {
			continue
		}

		region.Brightness = region.Deformation / float64(region.Counts)
	}

	return regions, nil
}

func (grid *Grid) ensurePrior(source, symbol string) *Prior {
	if grid.Priors[source] == nil {
		grid.Priors[source] = make(map[string]*Prior)
	}

	prior := grid.Priors[source][symbol]
	if prior == nil {
		prior = &Prior{}
		grid.Priors[source][symbol] = prior
	}

	return prior
}

func (grid *Grid) ingestSignal(
	slots *[RegionCount]*Region,
	regions *[]*Region,
	source, symbol string,
	labels map[string]float64,
) error {
	names := make([]string, 0, len(labels))

	for label := range labels {
		names = append(names, label)
	}

	slices.Sort(names)

	values := make([]float64, len(names))
	for i, label := range names {
		values[i] = labels[label]
	}

	signal := Signal{
		Source: source,
		Symbol: symbol,
		Labels: names,
		Values: values,
	}

	if err := pinned(symbol, signal); err != nil {
		return err
	}

	prior := grid.ensurePrior(source, symbol)

	if prior.Samples == 0 && len(prior.Signal.Labels) == 0 {
		prior.Signal = signal
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
	sourcePins, ok := regionMetricPins[source]

	if !ok {
		prior.Signal = signal
		prior.Energy = energy
		prior.Samples++
		return nil
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

			region := slots[key]

			if region == nil {
				region = &Region{Symbol: symbol, Key: key}
				slots[key] = region
				*regions = append(*regions, region)
			}

			region.Deformation += math.Abs(grid.SNM(&signal, idx)) * excitation
			region.Counts++
		}
	}

	prior.Signal = signal
	prior.Energy = energy
	prior.Samples++

	return nil
}

/*
pinned rejects a label that the source map does not name. An unknown source
has no pins to check. The error is returned to the caller and is not stored
on the grid.
*/
func pinned(symbol string, signal Signal) error {
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

/*
SNM (Self-Normalized Movement) takes the reference from the prior magnitude
and the movement itself. It measures a transition, not an isolated value.
The result stays within \([-1,1]\) without statistical calibration.
*/
func (grid *Grid) SNM(signal *Signal, idx int) float64 {
	prior := grid.ensurePrior(signal.Source, signal.Symbol)

	if idx >= len(signal.Values) || idx >= len(prior.Signal.Values) {
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
Surprise is the absolute gap between this tick's metric energy and the
running prediction. It is zero when they match. An empty prediction
returns the energy itself, which is a sum of squares and cannot be negative.
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
func (grid *Grid) Maturity(signal *Signal) float64 {
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
