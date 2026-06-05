package numeric

import (
	"math"

	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/numeric/adaptive"
	"github.com/theapemachine/symm/rawdump"
)

/*
CalibratorConfig holds shared online self-calibration defaults for any signal.
*/
type CalibratorConfig struct {
	Window     int
	RefitEvery int
	MinSamples int
	Blend      float64
	SeedField  string
}

/*
DefaultCalibratorConfig returns the platform-wide calibrator defaults.
*/
func DefaultCalibratorConfig(seedField string) CalibratorConfig {
	return CalibratorConfig{
		Window:     8192,
		RefitEvery: 256,
		MinSamples: 512,
		Blend:      0.3,
		SeedField:  seedField,
	}
}

/*
SignalCalibrator bundles one pooled classifier and calibrator for a signal.
*/
type SignalCalibrator struct {
	Classifier *adaptive.Classifier
	Calibrator *BandCalibrator
}

/*
NewSignalCalibrator builds a pooled calibrator, optionally warm-started from the
signal's most recent raw JSONL dump.
*/
func NewSignalCalibrator(
	seedEdges []float64,
	codes []float64,
	labels []string,
	targetShares []float64,
	config CalibratorConfig,
	dumpName string,
) *SignalCalibrator {
	calibrator := NewBandCalibrator(
		targetShares,
		config.Window,
		config.RefitEvery,
		config.MinSamples,
		config.Blend,
		perspectives.CurrentRegime,
	)

	if calibrator == nil {
		return nil
	}

	classifier := adaptive.NewClassifier(seedEdges, codes, labels)

	if dumpName != "" && config.SeedField != "" {
		SeedCalibratorFromDump(calibrator, classifier, dumpName, config.SeedField)
	}

	return &SignalCalibrator{
		Classifier: classifier,
		Calibrator: calibrator,
	}
}

/*
Observe feeds one pooled observation and refits band edges against the live regime.
*/
func (signalCalibrator *SignalCalibrator) Observe(observation float64) {
	if signalCalibrator == nil || signalCalibrator.Calibrator == nil {
		return
	}

	signalCalibrator.Calibrator.Observe(observation, signalCalibrator.Classifier)
}

/*
Telemetry returns the live calibration snapshot for dashboard gauges.
*/
func (signalCalibrator *SignalCalibrator) Telemetry(observation float64) Telemetry {
	if signalCalibrator == nil || signalCalibrator.Calibrator == nil {
		return Telemetry{}
	}

	telemetry := signalCalibrator.Calibrator.Snapshot(signalCalibrator.Classifier)
	telemetry.Observation = observation
	telemetry.EntropyTrust = EntropyTrustFromShares(telemetry.Shares)

	return telemetry
}

/*
AdjustStandout scales standout by entropy trust so flat category mixes reduce SNR.
*/
func (signalCalibrator *SignalCalibrator) AdjustStandout(standout float64, shares []float64) float64 {
	return standout * EntropyTrustFromShares(shares)
}

/*
ObserveGaugeTelemetry feeds one observation into the pooled calibrator, snapshots
dashboard telemetry, and scales standout by category-mix entropy trust.
*/
func ObserveGaugeTelemetry(
	calibrator *BandCalibrator,
	classifier *adaptive.Classifier,
	observation float64,
	standout float64,
) (Telemetry, float64) {
	if calibrator == nil || classifier == nil {
		return Telemetry{Observation: observation}, standout
	}

	calibrator.Observe(observation, classifier)
	telemetry := calibrator.Snapshot(classifier)
	telemetry.Observation = observation

	return telemetry, EntropyTrustFromShares(telemetry.Shares) * standout
}

/*
GaugePayload builds the dashboard gauge wire frame for a self-calibrating signal.
*/
func GaugePayload(
	source string,
	symbol string,
	category perspectives.CategoryType,
	measurement perspectives.Measurement,
	telemetry Telemetry,
) map[string]any {
	return map[string]any{
		"chart":         "gauge",
		"source":        source,
		"symbol":        symbol,
		"category":      category,
		"confidence":    measurement.Confidence,
		"snr":           measurement.SNR,
		"observation":   telemetry.Observation,
		"bands":         telemetry.Edges,
		"band_labels":   telemetry.Labels,
		"shares":        telemetry.Shares,
		"calibrating":   telemetry.Calibrating,
		"calibrated":    telemetry.Calibrated,
		"samples":       telemetry.Samples,
		"min_samples":   telemetry.MinSamples,
		"entropy_trust": telemetry.EntropyTrust,
	}
}

/*
SeedCalibratorFromDump preloads the rolling window from the most recent raw dump.
*/
func SeedCalibratorFromDump(
	calibrator *BandCalibrator,
	classifier *adaptive.Classifier,
	dumpName string,
	jsonField string,
) {
	if calibrator == nil || classifier == nil {
		return
	}

	observations, err := rawdump.ObservationSeed(dumpName, jsonField, calibrator.WindowCap())

	if err != nil || len(observations) == 0 {
		return
	}

	calibrator.SeedFromObservations(classifier, observations)
}

/*
RegimeTargetShares shifts category occupancy targets with the price-action regime.
Trending markets allow more top-tier categories; choppy markets flatten toward noise.
*/
func RegimeTargetShares(base []float64, regime perspectives.Regime) []float64 {
	if len(base) < 2 {
		return append([]float64(nil), base...)
	}

	out := append([]float64(nil), base...)

	switch regime {
	case perspectives.RegimeBullish, perspectives.RegimeTrending:
		if len(out) >= 2 {
			shift := 0.05
			out[len(out)-1] += shift
			out[0] = math.Max(0, out[0]-shift)
		}
	case perspectives.RegimeChoppy:
		if len(out) >= 3 {
			shift := 0.05
			middle := len(out) / 2
			out[middle] += shift
			out[0] = math.Max(0, out[0]-shift/2)
			out[len(out)-1] = math.Max(0, out[len(out)-1]-shift/2)
		}
	case perspectives.RegimeBearish:
		if len(out) >= 2 {
			shift := 0.04
			out[0] += shift
			out[len(out)-1] = math.Max(0, out[len(out)-1]-shift)
		}
	}

	return normalizeShares(out)
}

/*
RegimeBlend increases edge damping in choppy regimes to prevent category flicker.
*/
func RegimeBlend(baseBlend float64, regime perspectives.Regime) float64 {
	switch regime {
	case perspectives.RegimeChoppy:
		return math.Min(0.95, baseBlend+0.25)
	case perspectives.RegimeDead:
		return math.Min(0.90, baseBlend+0.15)
	default:
		return baseBlend
	}
}

/*
EntropyTrustFromShares returns [0,1]: 1 when one category dominates, lower when the
mix is uniform and classifications are less trustworthy.
*/
func EntropyTrustFromShares(shares []float64) float64 {
	if len(shares) < 2 {
		return 1
	}

	entropy := 0.0

	for _, share := range shares {
		if share <= 0 {
			continue
		}

		entropy -= share * math.Log2(share)
	}

	maxEntropy := math.Log2(float64(len(shares)))

	if maxEntropy <= 0 {
		return 1
	}

	return math.Max(0, 1-entropy/maxEntropy)
}

func normalizeShares(shares []float64) []float64 {
	if len(shares) == 0 {
		return nil
	}

	out := append([]float64(nil), shares...)
	total := 0.0

	for _, share := range out {
		total += share
	}

	if total <= 0 {
		return out
	}

	for index := range out {
		out[index] /= total
	}

	return out
}
