package store

import (
	"context"
	"math"
	"slices"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/types"
)

type Prior struct {
	Signal     Signal
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
of its self-normalized metric magnitudes |SNM| scaled by one plus the energy
surprise, Counts how many metrics it holds, and Brightness that average.
Brightness is not capped: the scale factor can push it above 1. The JSON
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
Observe applies every signal layer for symbol on this tick, then returns the
lit regions at or above the brightness midpoint.
*/
func (grid *Grid) Observe(value *data.Value) ([]*Region, error) {
	if value == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.NotFound,
			"[grid.Observe] nil value",
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

	var byKey [RegionCount]*Region
	regions := make([]*Region, 0, RegionCount)

	signals := map[string]map[string]Signal{}

	for _, source := range types.SignalSourceStrings {
		layers := value.LookupPrefix(source)

		for _, layer := range layers {
			parts := strings.Split(layer.Key, "/")
			if len(parts) < 3 {
				continue
			}

			var values []float64

			if err := layer.Decode(&values); err != nil {
				return nil, errnie.Error(errnie.Err(
					errnie.UnprocessableContent,
					"[grid.Observe] "+layer.Key+" does not decode",
					err,
				))
			}

			symbol := parts[1] + "/" + parts[2]

			if signals[source] == nil {
				signals[source] = make(map[string]Signal)
			}

			signals[source][symbol] = Signal{
				Source: source,
				Symbol: symbol,
				Labels: parts[3:],
				Values: values,
			}
		}
	}

	for source, bySource := range signals {
		for symbol, signal := range bySource {
			if err := pinned(symbol, signal); err != nil {
				return nil, err
			}

			if grid.Priors[source] == nil {
				grid.Priors[source] = make(map[string]*Prior)
			}

			prior := grid.Priors[source][symbol]

			if prior == nil {
				grid.Priors[source][symbol] = &Prior{Signal: signal}
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
			sourcePins, ok := regionMetricPins[source]

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
						region = &Region{Symbol: symbol, Key: key}
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
	}

	return grid.finalizeRegions(regions), nil
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

func (grid *Grid) finalizeRegions(regions []*Region) []*Region {
	if len(regions) == 0 {
		return nil
	}

	bySymbol := make(map[string][]*Region)
	order := make([]string, 0)

	for _, region := range regions {
		if _, seen := bySymbol[region.Symbol]; !seen {
			order = append(order, region.Symbol)
		}

		bySymbol[region.Symbol] = append(bySymbol[region.Symbol], region)
	}

	slices.Sort(order)

	kept := make([]*Region, 0, len(regions))

	for _, symbol := range order {
		kept = append(kept, cutMidpoint(bySymbol[symbol])...)
	}

	if len(kept) == 0 {
		return nil
	}

	return kept
}

/*
cutMidpoint keeps the regions at or above this symbol's own brightness
midpoint. An aggregate tick carries many symbols; a quiet name stays lit
when another name is louder.
*/
func cutMidpoint(regions []*Region) []*Region {
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
RegionVote is the side a confluence region speaks for, and whether it
votes with that side. The groups are the ones in SIGNALS.md.

Detector and support vote for their side: R00 and R01 for enter, R03 and
R04 for exit. Opposition votes against its side: R02 against enter, R05
against exit. Regime, history, and risk oracles (R06–R14) are context.
They return ("", false) and do not open or close a position.
*/
func RegionVote(key uint8) (side string, with bool) {
	switch key {
	case 0, 1:
		return "enter", true
	case 2:
		return "enter", false
	case 3, 4:
		return "exit", true
	case 5:
		return "exit", false
	default:
		return "", false
	}
}

/*
RegionSide is the side a region votes for. Opposition and oracles return
"" because they do not cast a vote for that side.
*/
func RegionSide(key uint8) string {
	side, with := RegionVote(key)
	if !with {
		return ""
	}

	return side
}

/*
SNM (Self-Normalized Movement) takes the reference from the prior magnitude
and the movement itself. It measures a transition, not an isolated value.
The result stays within \([-1,1]\) without statistical calibration.
*/
func (grid *Grid) SNM(signal *Signal, idx int) float64 {
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
SymbolSurprise is the mean absolute energy error of every source that has
a prior for symbol. The bool is false when no source has one yet: a missing
prior is not a surprise of zero.
*/
func (grid *Grid) SymbolSurprise(symbol string) (float64, bool) {
	if grid == nil || symbol == "" {
		return 0, false
	}

	var sum float64
	n := 0

	for _, bySymbol := range grid.Priors {
		prior := bySymbol[symbol]
		if prior == nil || prior.Samples <= 0 {
			continue
		}

		sum += grid.Surprise(prior.Prediction, prior.Energy)
		n++
	}

	if n == 0 {
		return 0, false
	}

	return sum / float64(n), true
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
