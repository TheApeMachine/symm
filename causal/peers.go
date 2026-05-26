package causal

import (
	"math"
	"sort"
)

const (
	minPeerSamples = 8
	maxPeerLinks   = 4
)

type peerLink struct {
	Symbol      string
	Correlation float64
}

/*
peerLinks ranks other symbols by Pearson correlation of recent price-velocity history.
*/
func (causal *Causal) peerLinks(focus string) []map[string]any {
	focusState := causal.symbols[focus]

	if focusState == nil || len(focusState.samples) < minPeerSamples {
		return nil
	}

	links := make([]peerLink, 0, len(causal.symbols))

	for symbol, state := range causal.symbols {
		if symbol == focus || len(state.samples) < minPeerSamples {
			continue
		}

		correlation := linkedVelocityCorrelation(focusState.samples, state.samples)

		if !finiteCorrelation(correlation) || correlation == 0 {
			continue
		}

		links = append(links, peerLink{
			Symbol:      symbol,
			Correlation: correlation,
		})
	}

	if len(links) == 0 {
		return nil
	}

	sort.Slice(links, func(left, right int) bool {
		return math.Abs(links[left].Correlation) > math.Abs(links[right].Correlation)
	})

	if len(links) > maxPeerLinks {
		links = links[:maxPeerLinks]
	}

	out := make([]map[string]any, 0, len(links))

	for _, link := range links {
		out = append(out, map[string]any{
			"symbol":      link.Symbol,
			"correlation": link.Correlation,
		})
	}

	return out
}

func linkedVelocityCorrelation(left, right []causalSample) float64 {
	leftVel := velocityTail(left, minPeerSamples)
	rightVel := velocityTail(right, minPeerSamples)
	overlap := min(len(leftVel), len(rightVel))

	if overlap < minPeerSamples {
		return 0
	}

	return pearson(
		leftVel[len(leftVel)-overlap:],
		rightVel[len(rightVel)-overlap:],
	)
}

func velocityTail(samples []causalSample, cap int) []float64 {
	start := len(samples) - cap

	if start < 0 {
		start = 0
	}

	vels := make([]float64, 0, len(samples)-start)

	for _, sample := range samples[start:] {
		vels = append(vels, sample.priceVelocity)
	}

	return vels
}

func finiteCorrelation(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
