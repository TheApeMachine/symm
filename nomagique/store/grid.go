package store

import (
	"fmt"
	"math"
	"strings"

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
noEvidence is the region index of a frame in which no region received a
single metric. Index 0 is never a pinned region, so the token "R00" states
the absence explicitly instead of crediting a real region.
*/
const noEvidence uint8 = 0

/*
unpinned is what PinRegion answers for a (source, metric) that belongs to no
confluence region: the cognitive and physical solvers (resonance, manifold),
which are model outputs rather than market observations, and any source the
map does not name. Unpinned metrics are counted but never scored, so an
unknown source cannot silently light a real region.
*/
const unpinned uint8 = 0

/*
Regions is one frame's region evidence. Brightness and Counts are indexed
by region (1..12; index 0 is unused). Unpinned counts the metrics that
belong to no region and were therefore not scored. Undefined counts the pinned
metrics whose z-score is not yet defined (Metric.Standardizable is false);
they are missing evidence, so they are excluded from their region's N rather
than scored as zero deformation. Winner and RunnerUp are
the brightest and second-brightest regions holding evidence; either is
noEvidence when no such region exists.
*/
type Regions struct {
	Brightness [13]float64
	Counts     [13]int
	Unpinned   int
	Undefined  int
	Winner     uint8
	RunnerUp   uint8
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
	var deformation [13]float64

	if measurement == nil {
		return regions
	}

	pin := func(source string, frame *data.Measurement) {
		for metric := range frame.Read() {
			if metric == nil || metric.Metric == nil {
				continue
			}

			region := grid.PinRegion(source, metric.Key)

			if region == unpinned {
				regions.Unpinned++
				continue
			}

			if !metric.Metric.Standardizable() {
				regions.Undefined++
				continue
			}

			deformation[region] += math.Abs(metric.Metric.Standardized)
			regions.Counts[region]++
		}
	}

	pin(measurement.Source, measurement)

	for _, peer := range measurement.Peers() {
		if peer == nil {
			continue
		}

		pin(peer.Source, peer)
	}

	for region := uint8(1); region <= 12; region++ {
		count := float64(regions.Counts[region])

		if count == 0 {
			continue
		}

		regions.Brightness[region] = (deformation[region] - count*halfNormalMean) /
			math.Sqrt(count*halfNormalVariance)

		regions.rank(region)
	}

	return regions
}

/*
rank places an evidenced region into the Winner/RunnerUp order.
*/
func (regions *Regions) rank(region uint8) {
	brightness := regions.Brightness[region]

	if regions.Winner == noEvidence || brightness > regions.Brightness[regions.Winner] {
		regions.RunnerUp = regions.Winner
		regions.Winner = region
		return
	}

	if regions.RunnerUp == noEvidence || brightness > regions.Brightness[regions.RunnerUp] {
		regions.RunnerUp = region
	}
}

/*
Observe returns the token of the brightest region ("R01" ... "R12"), or
"R00" when no region received any metric. The token is zero-padded for S3
path compatibility.
*/
func (grid *Grid) Observe(measurement *data.Measurement) []byte {
	return grid.RegionScores(measurement).Token()
}

/*
Token is the frame's token: its Winner, zero-padded ("R00" for no evidence).
*/
func (regions Regions) Token() []byte {
	return fmt.Appendf(nil, "R%02d", regions.Winner)
}

/*
PinRegion maps (source, metric) to its 12-tile confluence region, or to
unpinned for the solver sources (resonance, manifold) and any source it does
not name.
*/
func (grid *Grid) PinRegion(source, metric string) uint8 {
	m := strings.ToLower(metric)
	s := strings.ToLower(source)

	switch s {
	case "hawkes", "level3":
		if strings.Contains(m, ":buy") || strings.Contains(m, "buy_from_buy") {
			return 1 // R01: Buy Arrival Cascade
		}

		if strings.Contains(m, ":sell") || strings.Contains(m, "sell_from_sell") {
			return 3 // R03: Sell Arrival Cascade
		}

		return 2 // R02: Clock Acceleration / Branching

	case "pumpdump":
		if strings.Contains(m, "positive") {
			return 1 // R01
		}

		if strings.Contains(m, "negative") {
			return 3 // R03
		}

		return 2 // R02

	case "cvd", "spot:trade":
		if strings.Contains(m, ":buy") || (strings.Contains(m, "signed_net_fraction") && !strings.Contains(m, "divergence")) {
			return 4 // R04: Aggressive Taker Buy Inflow
		}

		if strings.Contains(m, ":sell") {
			return 6 // R06: Aggressive Taker Sell Outflow
		}

		return 5 // R05: Gross Churn / Two-Sided Volume

	case "depthflow", "toxicity", "liquidity":
		// Asks retreating = Offer vacuum
		if strings.Contains(m, ":ask") && (strings.Contains(m, "retreat") || strings.Contains(m, "removed")) {
			return 7 // R07: Ask Void
		}

		// Bids retreating = Waterfall
		if strings.Contains(m, ":bid") && (strings.Contains(m, "retreat") || strings.Contains(m, "removed")) {
			return 9 // R09: Bid Pulling
		}

		return 8 // R08: Spread / Friction / Turnover

	case "morphology":
		return 11 // R11: Iceberg Absorption Wall / Geometric Distortion

	case "sentiment", "correlation", "leadlag":
		if strings.Contains(m, "advance") || strings.Contains(m, "consensus") {
			return 10 // R10: Cohort Advance
		}

		if strings.Contains(m, "decline") || strings.Contains(m, "dispersion") {
			return 12 // R12: Cohort Decline
		}

		return 11 // R11
	}

	return unpinned
}
