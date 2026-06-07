package numeric

import (
	"math"
	"sort"

	"github.com/theapemachine/symm/market/perspectives/types"
	"github.com/theapemachine/symm/numeric/adaptive"
	"github.com/theapemachine/symm/ring"
)

/*
RegimeProvider supplies the price-action regime for band-edge retuning.
*/
type RegimeProvider func() types.Regime

/*
NewBandCalibrator builds a standalone calibrator that one or more pipes can share.
Pool a whole signal's symbols into a single calibrator so the bands — and the
dashboard's view of them — reflect one coherent distribution with one sample
count, instead of fragmenting into a separate, constantly-resetting state per
symbol. Returns nil for invalid parameters.
*/
func NewBandCalibrator(
	shares []float64,
	window, every, minSamples int,
	blend float64,
	regimeProvider RegimeProvider,
) *BandCalibrator {
	if window <= 0 || every <= 0 || len(shares) < 2 {
		return nil
	}

	if minSamples <= 0 {
		minSamples = window
	}

	shareEvery := min(every, 32)

	return &BandCalibrator{
		baseShares:     append([]float64(nil), shares...),
		shares:         append([]float64(nil), shares...),
		window:         ring.NewFloatRing(window),
		every:          every,
		minN:           minSamples,
		baseBlend:      blend,
		blend:          blend,
		shareEvery:     shareEvery,
		regimeProvider: regimeProvider,
	}
}

/*
Snapshot reports the calibrator's live state against classifier, for telemetry.
*/
func (calibrator *BandCalibrator) Snapshot(classifier *adaptive.Classifier) Telemetry {
	telemetry := Telemetry{}

	if calibrator == nil {
		return telemetry
	}

	if classifier != nil {
		telemetry.Edges = classifier.Upper()
		telemetry.Labels = classifier.Labels()
	}

	telemetry.Calibrated = calibrator.refits > 0
	telemetry.Calibrating = !telemetry.Calibrated
	telemetry.Samples = calibrator.window.Len()
	telemetry.MinSamples = calibrator.minN
	telemetry.Shares = append([]float64(nil), calibrator.recentShares...)
	telemetry.EntropyTrust = EntropyTrustFromShares(telemetry.Shares)

	return telemetry
}

/*
BandCalibrator keeps a rolling window of recent observations and refits the
classifier's band edges to canonical target-share quantiles on a cadence.
*/
type BandCalibrator struct {
	baseShares     []float64
	shares         []float64
	window         ring.FloatRing
	every          int
	minN           int
	baseBlend      float64
	blend          float64
	seen           int
	refits         int
	shareEvery     int
	recentShares   []float64
	regimeProvider RegimeProvider
}

/*
WindowCap returns the rolling observation capacity.
*/
func (calibrator *BandCalibrator) WindowCap() int {
	if calibrator == nil {
		return 0
	}

	return calibrator.window.Cap()
}

func (calibrator *BandCalibrator) activeRegime() types.Regime {
	if calibrator == nil || calibrator.regimeProvider == nil {
		return types.RegimeNone
	}

	return calibrator.regimeProvider()
}

func (calibrator *BandCalibrator) activeShares() []float64 {
	return RegimeTargetShares(calibrator.baseShares, calibrator.activeRegime())
}

func (calibrator *BandCalibrator) activeBlend() float64 {
	return RegimeBlend(calibrator.baseBlend, calibrator.activeRegime())
}

/*
SeedFromObservations preloads the rolling window and performs one refit when enough
prior observations exist. Used to warm-start from raw JSONL dumps at boot.
*/
func (calibrator *BandCalibrator) SeedFromObservations(
	classifier *adaptive.Classifier,
	observations []float64,
) {
	if calibrator == nil || classifier == nil || len(observations) == 0 {
		return
	}

	for _, observation := range observations {
		calibrator.window.Push(observation)
		calibrator.seen++
	}

	if calibrator.window.Len() < calibrator.minN {
		calibrator.recentShares = bandShares(calibrator.window.Ordered(), classifier.Upper())

		return
	}

	sorted := append([]float64(nil), calibrator.window.Ordered()...)
	sort.Float64s(sorted)

	fit := quantileBands(sorted, calibrator.activeShares())

	if len(fit) == 0 {
		return
	}

	if calibrator.activeBlend() > 0 {
		fit = blendEdges(classifier.Upper(), fit, calibrator.activeBlend())
	}

	classifier.SetUpper(fit)
	calibrator.refits++
	calibrator.shares = calibrator.activeShares()
	calibrator.blend = calibrator.activeBlend()
	calibrator.recentShares = bandShares(sorted, fit)
}

/*
Observe absorbs one observation and refits band edges when needed.
*/
func (calibrator *BandCalibrator) Observe(observation float64, classifier *adaptive.Classifier) {
	calibrator.window.Push(observation)
	calibrator.seen++

	calibrator.shares = calibrator.activeShares()
	calibrator.blend = calibrator.activeBlend()

	if calibrator.shareEvery > 0 && calibrator.seen%calibrator.shareEvery == 0 && calibrator.window.Len() > 0 {
		calibrator.recentShares = bandShares(calibrator.window.Ordered(), classifier.Upper())
	}

	if calibrator.seen%calibrator.every != 0 || calibrator.window.Len() < calibrator.minN {
		return
	}

	sorted := append([]float64(nil), calibrator.window.Ordered()...)
	sort.Float64s(sorted)

	fit := quantileBands(sorted, calibrator.shares)

	if len(fit) == 0 {
		return
	}

	if calibrator.blend > 0 {
		fit = blendEdges(classifier.Upper(), fit, calibrator.blend)
	}

	classifier.SetUpper(fit)
	calibrator.refits++
	calibrator.recentShares = bandShares(sorted, fit)
}

func bandShares(values []float64, edges []float64) []float64 {
	counts := make([]int, len(edges)+1)

	for _, value := range values {
		counts[sort.SearchFloat64s(edges, value)]++
	}

	shares := make([]float64, len(counts))

	if len(values) == 0 {
		return shares
	}

	for index, count := range counts {
		shares[index] = float64(count) / float64(len(values))
	}

	return shares
}

func quantileBands(sorted []float64, shares []float64) []float64 {
	if len(shares) < 2 || len(sorted) == 0 {
		return nil
	}

	total := 0.0

	for _, share := range shares {
		total += share
	}

	if total <= 0 {
		total = 1
	}

	edges := make([]float64, 0, len(shares)-1)
	cumulative := 0.0

	for i := 0; i < len(shares)-1; i++ {
		cumulative += shares[i] / total
		edges = append(edges, quantileSorted(sorted, cumulative))
	}

	return ascendingDistinct(edges)
}

func quantileSorted(sorted []float64, fraction float64) float64 {
	if len(sorted) == 0 {
		return 0
	}

	if len(sorted) == 1 {
		return sorted[0]
	}

	index := int(math.Round(fraction * float64(len(sorted)-1)))

	if index < 0 {
		index = 0
	}

	if index >= len(sorted) {
		index = len(sorted) - 1
	}

	return sorted[index]
}

func blendEdges(old, fit []float64, blend float64) []float64 {
	if len(old) != len(fit) {
		return fit
	}

	out := make([]float64, len(fit))

	for i := range fit {
		out[i] = blend*old[i] + (1-blend)*fit[i]
	}

	return ascendingDistinct(out)
}

func ascendingDistinct(edges []float64) []float64 {
	for i := 1; i < len(edges); i++ {
		if edges[i] <= edges[i-1] {
			edges[i] = math.Nextafter(edges[i-1], math.Inf(1))
		}
	}

	return edges
}
