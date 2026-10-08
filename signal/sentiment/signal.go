package sentiment

import (
	"context"
	"math"
	"slices"
	"sync"
	"unsafe"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/transport"
)

var outputKeys = []string{
	"cohort_member_count",
	"valid_member_count",
	"excluded_member_count",
	"cohort_horizon_seconds",
	"return",
	"absolute_return",
	"asof_age_seconds",
	"from_age_seconds",
	"advance_count",
	"decline_count",
	"unchanged_count",
	"advance_fraction",
	"decline_fraction",
	"unchanged_fraction",
	"directional_participation",
	"breadth",
	"directional_agreement",
	"directional_consensus",
	"median_return",
	"median_absolute_return",
	"mean_absolute_return",
	"rms_return",
	"return_mad",
	"magnitude_mad",
	"return_interquartile_range",
	"largest_move_tie_count",
	"largest_absolute_return",
	"largest_signed_return",
	"largest_move_share",
	"peer_median_absolute_return",
	"peer_magnitude_mad",
	"largest_move_excess",
	"largest_move_ratio",
	"largest_move_mad_excess",
	"same_direction_peer_count",
	"opposite_direction_peer_count",
	"zero_return_peer_count",
	"same_direction_peer_fraction",
	"opposite_direction_peer_fraction",
	"zero_return_peer_fraction",
	"breadth_baseline",
	"breadth_divergence",
	"breadth_zscore",
	"median_return_baseline",
	"median_return_divergence",
	"median_return_zscore",
	"median_return_velocity",
	"breadth_velocity",
	"historical_path_distance",
	"historical_path_percentile",
}

type Signal struct {
	*runtime.System
	pipeline        *nomagique.Number
	prices          sync.Map
	prevPrices      sync.Map
	medianBaseline  core.Primitive
	breadthBaseline core.Primitive
	medianVel       core.Primitive
	breadthVel      core.Primitive
	mu              sync.Mutex
}

func NewSignal(ctx context.Context) *Signal {
	signal := &Signal{
		pipeline: nomagique.NewNumber(
			transport.NewAddressable(
				"symbolstore", store.NewKV(),
				nomagique.NewNumber(
					data.NewSelect(
						0, 1, 2, 3, 4, 5, 6, 7, 8, 9,
						10, 11, 12, 13, 14, 15, 16, 17, 18, 19,
						20, 21, 22, 23, 24, 25, 26, 27, 28, 29,
						30, 31, 32, 33, 34, 35, 36, 37, 38, 39,
						40, 41, 42, 43, 44, 45, 46, 47, 48, 49,
					),
				),
				data.NewMessage(data.WRITE, "symbolstore", "sentiment_state", data.NewValue[core.Primitive]()),
			),
		),
		medianBaseline:  adaptive.NewBaseline(adaptive.NewWindow()),
		breadthBaseline: adaptive.NewBaseline(adaptive.NewWindow()),
		medianVel:       temporal.NewVelocity(),
		breadthVel:      temporal.NewVelocity(),
	}

	signal.System = runtime.NewSystem(ctx, "sentiment", signal)
	return signal
}

func (signal *Signal) Step(prior *data.Measurement) *data.Measurement {
	if signal.Status() != runtime.READY || prior == nil || prior.Label == "" {
		return nil
	}

	priceEntry := data.Pull(prior.Read("price"))
	if priceEntry == nil || priceEntry.Metric == nil {
		return nil
	}
	price := priceEntry.Metric.Raw
	if price <= 0 || math.IsNaN(price) || math.IsInf(price, 0) {
		return nil
	}

	signal.mu.Lock()
	defer signal.mu.Unlock()

	prevPriceVal, hadPrevPrice := signal.prices.Load(prior.Label)
	signal.prices.Store(prior.Label, price)

	if hadPrevPrice {
		signal.prevPrices.Store(prior.Label, prevPriceVal.(float64))
	}

	var validCount, advanceCount, declineCount, unchangedCount float64
	var returns []float64

	signal.prices.Range(func(key, value any) bool {
		sym := key.(string)
		curr := value.(float64)
		if prev, ok := signal.prevPrices.Load(sym); ok {
			p := prev.(float64)
			if p > 0 {
				r := (curr - p) / p
				returns = append(returns, r)
				validCount++
				if r > 0 {
					advanceCount++
				} else if r < 0 {
					declineCount++
				} else {
					unchangedCount++
				}
			}
		}
		return true
	})

	var advanceFraction, declineFraction, unchangedFraction, breadth, medianReturn float64
	if validCount > 0 {
		advanceFraction = advanceCount / validCount
		declineFraction = declineCount / validCount
		unchangedFraction = unchangedCount / validCount
		breadth = (advanceCount - declineCount) / validCount

		slices.Sort(returns)
		n := len(returns)
		if n%2 == 1 {
			medianReturn = returns[n/2]
		} else {
			medianReturn = (returns[n/2-1] + returns[n/2]) / 2.0
		}
	}

	var medianCenter, medianScale, medianDivergence, medianZscore float64
	var breadthCenter, breadthScale, breadthDivergence, breadthZscore float64
	var medianVelocity, breadthVelocity float64
	atNano := float64(prior.At.UnixNano())

	if validCount > 0 {
		idx := 0
		for ptr := range signal.medianBaseline.Next(data.NewValue(medianReturn).Next(nil)) {
			if idx == 0 {
				medianCenter = *(*float64)(ptr)
			} else if idx == 1 {
				medianScale = *(*float64)(ptr)
			}
			idx++
		}
		medianDivergence = medianReturn - medianCenter
		if medianScale > 0 {
			medianZscore = medianDivergence / medianScale
		}

		for ptr := range signal.medianVel.Next(data.NewValue(medianReturn, atNano).Next(nil)) {
			medianVelocity = *(*float64)(ptr)
		}

		idx = 0
		for ptr := range signal.breadthBaseline.Next(data.NewValue(breadth).Next(nil)) {
			if idx == 0 {
				breadthCenter = *(*float64)(ptr)
			} else if idx == 1 {
				breadthScale = *(*float64)(ptr)
			}
			idx++
		}
		breadthDivergence = breadth - breadthCenter
		if breadthScale > 0 {
			breadthZscore = breadthDivergence / breadthScale
		}

		for ptr := range signal.breadthVel.Next(data.NewValue(breadth, atNano).Next(nil)) {
			breadthVelocity = *(*float64)(ptr)
		}
	}

	rawMetrics := []float64{
		validCount,
		validCount,
		0,
		0,
		0, // return
		0, // absolute return
		0, // asof age
		0, // from age
		advanceCount,
		declineCount,
		unchangedCount,
		advanceFraction,
		declineFraction,
		unchangedFraction,
		0, // directional participation
		breadth,
		0, // agreement
		0, // consensus
		medianReturn,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, // dispersion, largest moves, peer metrics
		breadthCenter,
		breadthDivergence,
		breadthZscore,
		medianCenter,
		medianDivergence,
		medianZscore,
		medianVelocity,
		breadthVelocity,
		0,
		0,
	}

	output := make(map[string]float64)

	for ptr := range signal.pipeline.Next(
		data.NewMessage(
			data.WRITE,
			"symbolstore",
			prior.Label,
			data.NewValue(
				unsafe.Pointer(&rawMetrics),
			),
		).Next(nil),
	) {
		if ptr == nil {
			continue
		}
	}

	for i, key := range outputKeys {
		if i < len(rawMetrics) {
			output[key] = rawMetrics[i]
		}
	}

	return prior.Next(signal.Name(), output)
}
