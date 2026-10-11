package store

import (
	"fmt"
	"math"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Grid is not a grid at all, it is a "group by" over Measurement
and Metric pins into 12 multi-signal confluence regions.
*/
type Grid struct{}

func NewGrid() *Grid {
	return &Grid{}
}

/*
Under the null hypothesis a standardized metric is z ~ N(0, 1), so its
deformation |z| is half-normal with mean sqrt(2/pi) and variance 1 - 2/pi.
These are exact identities of that distribution, not tuning constants.
*/
var (
	halfNormalMean     = math.Sqrt(2 / math.Pi)
	halfNormalVariance = 1 - 2/math.Pi
)

/*
Regions is one frame's region evidence. Brightness and Counts are indexed
by region 0..11 directly.
Winner and RunnerUp are the brightest and second-brightest regions holding
evidence (0..11) when HasEvidence is true.
*/
type Regions struct {
	Brightness  [12]float64
	Counts      [12]int
	HasEvidence bool
	Winner      uint8
	RunnerUp    uint8
}

/*
RegionScores pins every metric of the frame and its peers to a region and
scores each region by the standardized excess of its summed deformation:

	brightness = (sum|z| - N*sqrt(2/pi)) / sqrt(N*(1 - 2/pi))

Under the null every region's brightness has mean 0 and variance 1 whatever
its metric count N, so the argmax is not biased toward the regions that
happen to hold more metrics (sum|z|/sqrt(N) grows as sqrt(N) under the null
and lets the largest region win almost every frame). Brightness is negative
when a region is quieter than the null. Observe emits Winner, so the audit
and production share this one path.
*/
func (grid *Grid) RegionScores(measurement *data.Measurement) Regions {
	var regions Regions

	if measurement == nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"[grid.RegionScores] nil measurement provided",
			nil,
		))

		return regions
	}

	var deformation [12]float64
	var nullMean [12]float64
	var nullVariance [12]float64
	var failed bool

	pin := func(source string, frame *data.Measurement) {
		if failed || frame == nil {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"[grid.RegionScores] nil measurement provided",
				nil,
			))

			return
		}

		metrics := frame.Dampen()

		if len(metrics) == 0 {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"[grid.RegionScores] no metrics available",
				nil,
			), "source", source)

			return
		}

		confidence := frame.Confidence()

		if confidence <= 0 {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"[grid.RegionScores] measurement confidence is not positive",
				nil,
			), "source", source, "confidence", confidence)

			failed = true
			return
		}

		for _, metric := range metrics {
			region, ok := grid.PinRegion(source, metric.Label)

			if !ok {
				failed = true
				return
			}

			if !metric.Standardizable() {
				errnie.Error(errnie.Err(
					errnie.Validation,
					"[grid.RegionScores] metric is not standardizable",
					nil,
				), "source", source, "metric", metric.Label)

				failed = true
				return
			}

			deformation[region] += math.Abs(metric.Standardized)
			nullMean[region] += confidence * halfNormalMean
			nullVariance[region] += confidence * confidence * halfNormalVariance
			regions.Counts[region]++
		}
	}

	pin(measurement.Source, measurement)

	for _, peer := range measurement.Peers() {
		pin(peer.Source, peer)
	}

	if failed {
		regions.HasEvidence = false
		return regions
	}

	for region := range uint8(12) {
		if regions.Counts[region] == 0 || nullVariance[region] == 0 {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"[grid.RegionScores] region missing evidence",
				nil,
			), "region", region, "source", measurement.Source)

			regions.HasEvidence = false
			return regions
		}
	}

	for region := range uint8(12) {
		regions.Brightness[region] = (deformation[region] - nullMean[region]) /
			math.Sqrt(nullVariance[region])
	}

	winner := uint8(0)
	runnerUp := uint8(1)

	if regions.Brightness[1] > regions.Brightness[0] {
		winner = 1
		runnerUp = 0
	}

	for region := uint8(2); region < 12; region++ {
		score := regions.Brightness[region]

		if score > regions.Brightness[winner] {
			runnerUp = winner
			winner = region
			continue
		}

		if score > regions.Brightness[runnerUp] {
			runnerUp = region
		}
	}

	regions.Winner = winner
	regions.RunnerUp = runnerUp
	regions.HasEvidence = true
	return regions
}

/*
Observe returns the token of the brightest region ("R00" ... "R11"), or
nil when no region received any evidenced metric.
*/
func (grid *Grid) Observe(measurement *data.Measurement) []byte {
	return grid.RegionScores(measurement).Token()
}

/*
Token is the frame's token: its Winner, zero-padded ("R00" ... "R11"), or nil
when no region held evidenced metrics.
*/
func (regions Regions) Token() []byte {
	if !regions.HasEvidence {
		return nil
	}

	return fmt.Appendf(nil, "R%02d", regions.Winner)
}

/*
PinRegion maps (source, metric) to its 12 confluence regions 0..11, returning
the region and true when mapped, or 0 and false for solver sources (resonance,
manifold) and any source it does not name.
*/
func (grid *Grid) PinRegion(source, metric string) (uint8, bool) {
	m := strings.ToLower(metric)
	s := strings.ToLower(source)

	switch s {
	case "resonance", "manifold", "runtime:join":
		errnie.Error(errnie.Err(
			errnie.NotAcceptable,
			"[grid] metric leakage detected",
			nil,
		), "source", source, "metric", metric)

		return 0, false

	case "hawkes", "spot:level3":
		if strings.Contains(m, ":buy") || strings.Contains(m, "buy_from_buy") {
			return 0, true // R00: Buy Arrival Cascade
		}

		if strings.Contains(m, ":sell") || strings.Contains(m, "sell_from_sell") {
			return 2, true // R02: Sell Arrival Cascade
		}

		return 1, true // R01: Clock Acceleration / Branching

	case "pumpdump":
		if strings.Contains(m, "positive") {
			return 0, true // R00
		}

		if strings.Contains(m, "negative") {
			return 2, true // R02
		}

		return 1, true // R01

	case "cvd", "spot:trade":
		if strings.Contains(m, ":buy") || (strings.Contains(m, "signed_net_fraction") && !strings.Contains(m, "divergence")) {
			return 3, true // R03: Aggressive Taker Buy Inflow
		}

		if strings.Contains(m, ":sell") {
			return 5, true // R05: Aggressive Taker Sell Outflow
		}

		return 4, true // R04: Gross Churn / Two-Sided Volume

	case "depthflow", "toxicity", "liquidity":
		// Asks retreating = Offer vacuum
		if strings.Contains(m, ":ask") && (strings.Contains(m, "retreat") || strings.Contains(m, "removed")) {
			return 6, true // R06: Ask Void
		}

		// Bids retreating = Waterfall
		if strings.Contains(m, ":bid") && (strings.Contains(m, "retreat") || strings.Contains(m, "removed")) {
			return 8, true // R08: Bid Pulling
		}

		return 7, true // R07: Spread / Friction / Turnover

	case "morphology":
		return 10, true // R10: Iceberg Absorption Wall / Geometric Distortion

	case "sentiment", "correlation", "leadlag":
		if strings.Contains(m, "advance") || strings.Contains(m, "consensus") {
			return 9, true // R09: Cohort Advance
		}

		if strings.Contains(m, "decline") || strings.Contains(m, "dispersion") {
			return 11, true // R11: Cohort Decline
		}

		return 10, true // R10
	}

	errnie.Error(errnie.Err(
		errnie.Validation,
		"[grid.PinRegion] metric not mapped to region",
		nil,
	), "source", source, "metric", metric)

	return 0, false
}
