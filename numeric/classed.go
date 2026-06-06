package numeric

import (
	"math"
	"sort"

	"github.com/theapemachine/symm/market/perspectives/types"
	"github.com/theapemachine/symm/numeric/adaptive"
	"github.com/theapemachine/symm/ring"
)

type Classed struct {
	derived    *Derived
	classifier *adaptive.Classifier
	classify   *Classify
}

func NewClassed(classifier *adaptive.Classifier, stages ...Dynamic) *Classed {
	classify := NewClassify(classifier)

	return &Classed{
		derived:    NewDerived(WithDynamics(append(stages, classify)...)),
		classifier: classifier,
		classify:   classify,
	}
}

func (classed *Classed) Push(values ...float64) (float64, error) {
	return classed.derived.Push(values...)
}

func (classed *Classed) Label(code float64) string {
	return classed.classifier.Label(code)
}

/*
Confidence returns how clearly the last Push landed in its category — see
adaptive.Classifier.Confidence.
*/
func (classed *Classed) Confidence() float64 {
	if classed == nil || classed.classifier == nil {
		return 0
	}

	return classed.classifier.Confidence(classed.classify.lastObservation)
}

func (classed *Classed) Standout() float64 {
	if classed == nil || classed.classifier == nil {
		return 0
	}

	return classed.classifier.Standout(classed.classify.lastObservation)
}

/*
Observation returns the scalar the classifier banded on the last Push — the
post-projection, post-clamp value. It is what self-calibration fits the band
edges to, and what a raw dump records to make the signal observable.
*/
func (classed *Classed) Observation() float64 {
	if classed == nil || classed.classify == nil {
		return 0
	}

	return classed.classify.lastObservation
}

/*
Telemetry is a live snapshot of a self-calibrating classifier's state for the
dashboard: the current band edges, their labels, the recent category mix, the last
banded observation, and whether calibration has refit yet. It is produced by
BandCalibrator.Snapshot — pool a signal's symbols into one shared calibrator and
read this once per emit (see signal/pumpdump and signal/correlation).
*/
type Telemetry struct {
	Edges        []float64 `json:"bands"`
	Labels       []string  `json:"labels"`
	Shares       []float64 `json:"shares"`
	Observation  float64   `json:"observation"`
	Calibrating  bool      `json:"calibrating"`
	Calibrated   bool      `json:"calibrated"`
	Samples      int       `json:"samples"`
	MinSamples   int       `json:"min_samples"`
	EntropyTrust float64   `json:"entropy_trust"`
}

func (classed *Classed) Reset() error {
	return classed.derived.Reset()
}

type Classify struct {
	classifier      *adaptive.Classifier
	lastObservation float64
}

func NewClassify(classifier *adaptive.Classifier) *Classify {
	return &Classify{classifier: classifier}
}

func (classify *Classify) Next(out float64, values ...float64) (float64, error) {
	_ = values
	classify.lastObservation = out

	return classify.classifier.Code(out)
}

func (classify *Classify) Reset() error {
	return nil
}

/*
NewBandCalibrator builds a standalone calibrator that one or more pipes can share.
Pool a whole signal's symbols into a single calibrator so the bands — and the
dashboard's view of them — reflect one coherent distribution with one sample
count, instead of fragmenting into a separate, constantly-resetting state per
symbol. Returns nil for invalid parameters.
*/
/*
RegimeProvider supplies the price-action regime for band-edge retuning.
*/
type RegimeProvider func() types.Regime

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
	telemetry := Telemetry{Calibrating: true}

	if calibrator == nil {
		return telemetry
	}

	if classifier != nil {
		telemetry.Edges = classifier.Upper()
		telemetry.Labels = classifier.Labels()
	}

	telemetry.Calibrated = calibrator.refits > 0
	telemetry.Samples = calibrator.window.Len()
	telemetry.MinSamples = calibrator.minN
	telemetry.Shares = append([]float64(nil), calibrator.recentShares...)
	telemetry.EntropyTrust = EntropyTrustFromShares(telemetry.Shares)

	return telemetry
}

// BandCalibrator keeps a rolling window of recent observations and refits the
// classifier's band edges to canonical target-share quantiles on a cadence.
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

func (calibrator *BandCalibrator) Observe(observation float64, classifier *adaptive.Classifier) {
	calibrator.window.Push(observation)
	calibrator.seen++

	calibrator.shares = calibrator.activeShares()
	calibrator.blend = calibrator.activeBlend()

	// Refresh the live category mix under the CURRENT edges often, so the
	// dashboard always shows the real distribution — including during warm-up,
	// where it honestly reveals the seed edges have not adapted yet.
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

// bandShares reports the fraction of values that land in each band under edges.
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

// quantileBands places len(shares)-1 ascending band edges at the cumulative-share
// quantiles of a sorted observation series, so each category occupies its target
// share of the distribution.
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

// blendEdges damps edge movement: out = blend*old + (1-blend)*fit, kept ascending.
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
