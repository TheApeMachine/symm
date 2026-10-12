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
		if failed {
			return
		}

		if frame == nil {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"[grid.RegionScores] nil measurement provided",
				nil,
			))

			failed = true
			return
		}

		switch source {
		case "resonance", "manifold", "runtime:join":
			errnie.Error(errnie.Err(
				errnie.NotAcceptable,
				"[grid] metric leakage detected",
				nil,
			), "source", source)

			failed = true
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
			region, isPinned := grid.PinRegion(source, metric.Label)

			if !isPinned {
				continue
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

	for _, peer := range measurement.Peers() {
		if failed {
			break
		}

		pin(peer.Source, peer)
	}

	if failed {
		regions.HasEvidence = false
		return regions
	}

	hasAny := false

	for region := range uint8(12) {
		if regions.Counts[region] > 0 && nullVariance[region] > 0 {
			regions.Brightness[region] = (deformation[region] - nullMean[region]) /
				math.Sqrt(nullVariance[region])
			hasAny = true
		}
	}

	if !hasAny {
		regions.HasEvidence = false
		return regions
	}

	var winner uint8
	var runnerUp uint8
	winnerFound := false
	runnerUpFound := false

	for region := range uint8(12) {
		if regions.Counts[region] == 0 || nullVariance[region] == 0 {
			continue
		}

		if !winnerFound {
			winner = region
			winnerFound = true
			continue
		}

		score := regions.Brightness[region]

		if score > regions.Brightness[winner] {
			runnerUp = winner
			runnerUpFound = true
			winner = region
			continue
		}

		if !runnerUpFound || score > regions.Brightness[runnerUp] {
			runnerUp = region
			runnerUpFound = true
		}
	}

	if !runnerUpFound {
		runnerUp = winner
	}

	regions.Winner = winner
	regions.RunnerUp = runnerUp
	regions.HasEvidence = true
	return regions
}

/*
Ready reports whether a symbol's measurement frame (and its peers) has completed
warmup and is ready for grid evaluation: all pinned metric streams carry positive
confidence and valid causal standardization scales.
*/
func (grid *Grid) Ready(measurement *data.Measurement) bool {
	if measurement == nil {
		return false
	}

	frames := measurement.Peers()

	if len(frames) == 0 {
		frames = []*data.Measurement{measurement}
	}

	hasPinned := false

	for _, frame := range frames {
		if frame == nil {
			return false
		}

		metrics := frame.Dampen()

		if len(metrics) == 0 {
			return false
		}

		if frame.Confidence() <= 0 {
			return false
		}

		for _, metric := range metrics {
			_, isPinned := grid.PinRegion(frame.Source, metric.Label)

			if !isPinned {
				continue
			}

			if !metric.Standardizable() {
				return false
			}

			hasPinned = true
		}
	}

	return hasPinned
}

/*
Observe returns the token of the brightest region ("R00" ... "R11"), or
nil when no region received any evidenced metric.
*/
func (grid *Grid) Observe(measurement *data.Measurement) []byte {
	if !grid.Ready(measurement) {
		return nil
	}

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
manifold, runtime:join) and any metric not part of the confluence grid.
*/
func (grid *Grid) PinRegion(source, metric string) (uint8, bool) {
	lowerSource := strings.ToLower(source)
	lowerMetric := strings.ToLower(metric)

	switch lowerSource {
	case "resonance", "manifold", "runtime:join":
		errnie.Error(errnie.Err(
			errnie.NotAcceptable,
			"[grid] metric leakage detected",
			nil,
		), "source", source, "metric", metric)

		return 0, false
	}

	sourcePins, hasSource := regionMetricPins[lowerSource]

	if !hasSource {
		return 0, false
	}

	if region, isPinned := sourcePins[lowerMetric]; isPinned {
		return region, true
	}

	// Support test fixtures with indexed stems: stem_0, stem_1, etc.
	if separatorIndex := strings.LastIndex(lowerMetric, "_"); separatorIndex > 0 {
		stem := lowerMetric[:separatorIndex]

		if region, isPinned := sourcePins[stem]; isPinned {
			return region, true
		}
	}

	return 0, false
}
