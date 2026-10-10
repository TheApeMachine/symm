package audit

import (
	"fmt"
	"math"
	"math/rand"
	"sync/atomic"

	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/store"
)

/*
replayEpoch hands out synthetic, negative epochs. The standardizer keys its
streams by epoch, so each replay starts from empty moments and never touches
the stored epoch's or another replay's streams.
*/
var replayEpoch atomic.Int64

/*
replayPipeline turns an ordered list of stored measurements into one output
per (measurement, metric) plus one grid token per (symbol, tick). The key
identifies the stored measurement, so two runs can be compared observation by
observation.
*/
type replayPipeline func(measurements []*data.Measurement, epoch int64, grid *store.Grid) map[string]float64

func observationKey(m *data.Measurement, metric string) string {
	return fmt.Sprintf("%s|%s|%d|%d|%s", m.Source, m.Label, m.Tick, m.SeqIdx, metric)
}

/*
productionReplay re-finalizes every stored raw value through the production
data.Measurement.Write path (fresh standardizer streams in the replay epoch)
and lights the production grid on each symbol's re-standardized frames at
each tick, recording every z-score and token.
*/
func productionReplay(measurements []*data.Measurement, epoch int64, grid *store.Grid) map[string]float64 {
	outputs := make(map[string]float64)
	type frameKey struct {
		label string
		tick  int64
	}
	byFrame := make(map[frameKey][]*data.Measurement)
	var order []frameKey

	for _, stored := range measurements {
		if stored == nil {
			continue
		}

		replayed := data.NewMeasurement(epoch, stored.Label, stored.Source, stored.SeqIdx, stored.Tick)
		replayed.At, replayed.From = stored.At, stored.From
		metrics := make([]*data.Metric, 0)

		for entry := range stored.Read() {
			if entry != nil && entry.Metric != nil {
				metrics = append(metrics, data.NewMetric(entry.Key, entry.Metric.Raw, entry.Metric.Unit(), entry.Metric.Timescale()))
			}
		}

		replayed = replayed.Write(metrics...)

		for entry := range replayed.Read() {
			if entry != nil && entry.Metric != nil {
				outputs[observationKey(stored, entry.Key)] = entry.Metric.Standardized
			}
		}

		key := frameKey{stored.Label, stored.Tick}

		if _, seen := byFrame[key]; !seen {
			order = append(order, key)
		}

		byFrame[key] = append(byFrame[key], replayed)
	}

	if grid == nil {
		return outputs
	}

	for _, key := range order {
		peers := byFrame[key]
		frame := data.NewMeasurement(epoch, key.label, "causality", peers[0].SeqIdx, key.tick)
		frame.At, frame.From = peers[0].At, peers[0].From
		frame.Peers(peers...)
		frame.Write()

		if reg, ok := tokenRegion(grid.Observe(frame)); ok {
			outputs[fmt.Sprintf("token|%s|%d", key.label, key.tick)] = float64(reg)
		}
	}

	return outputs
}

/*
perturbFuture returns copies of future with every raw value scaled by an
independent random factor in (0, 2), keeping identities and order. A causal
pipeline's outputs for earlier observations cannot depend on these values.
*/
func perturbFuture(future []*data.Measurement, seed int64) []*data.Measurement {
	rng := rand.New(rand.NewSource(seed))
	out := make([]*data.Measurement, 0, len(future))

	for _, stored := range future {
		if stored == nil {
			continue
		}

		copyOf := data.NewMeasurement(stored.Epoch, stored.Label, stored.Source, stored.SeqIdx, stored.Tick)
		copyOf.At, copyOf.From = stored.At, stored.From
		metrics := make([]*data.Metric, 0)

		for entry := range stored.Read() {
			if entry != nil && entry.Metric != nil {
				metrics = append(metrics, data.NewMetric(
					entry.Key, entry.Metric.Raw*2*rng.Float64(), entry.Metric.Unit(), entry.Metric.Timescale(),
				))
			}
		}

		out = append(out, copyOf.Write(metrics...))
	}

	return out
}

/*
differing counts the keys of reference whose value in other differs (both
undefined-as-zero and NaN compare equal to themselves).
*/
func differing(reference, other map[string]float64) int {
	count := 0

	for key, value := range reference {
		got, ok := other[key]

		if !ok || (got != value && !(math.IsNaN(got) && math.IsNaN(value))) {
			count++
		}
	}

	return count
}

/*
causalityProbe runs pipeline three ways and compares outputs observation by
observation:
 1. future: past alone versus past followed by a perturbed future; every
    past output must be unchanged.
 2. cross-symbol: one symbol's measurements alone versus interleaved with
    every other symbol; that symbol's outputs must be unchanged.
 3. epoch: the same past replayed in two fresh epochs must give identical
    outputs, so no state survives across epochs.
*/
func causalityProbe(pipeline replayPipeline, grid *store.Grid, past, future []*data.Measurement, seed int64) CausalityAudit {
	report := CausalityAudit{FuturePerturbationTicks: len(future)}
	alone := pipeline(past, replayEpoch.Add(-1), grid)
	extended := pipeline(append(append([]*data.Measurement(nil), past...), perturbFuture(future, seed)...),
		replayEpoch.Add(-1), grid)

	report.ComparedObservations = len(alone)
	report.ContaminatedCount = differing(alone, extended)
	report.LeakageDetected = report.ContaminatedCount > 0

	all := append(append([]*data.Measurement(nil), past...), future...)
	var target string

	for _, m := range all {
		if m != nil && m.Label != "" {
			target = m.Label
			break
		}
	}

	symbols := make(map[string]struct{})
	var only []*data.Measurement

	for _, m := range all {
		if m == nil {
			continue
		}

		symbols[m.Label] = struct{}{}

		if m.Label == target {
			only = append(only, m)
		}
	}

	report.SymbolsTested = len(symbols)

	if len(symbols) >= 2 {
		isolated := pipeline(only, replayEpoch.Add(-1), grid)
		interleaved := pipeline(all, replayEpoch.Add(-1), grid)
		report.CrossSymbolContaminated = differing(isolated, interleaved)
		report.CrossSymbolLeakage = report.CrossSymbolContaminated > 0
	}

	again := pipeline(past, replayEpoch.Add(-1), grid)
	report.EpochContaminated = differing(alone, again)
	report.EpochIsolationPassed = report.EpochContaminated == 0

	verdict := VerdictValid

	switch {
	case report.ComparedObservations == 0 || len(future) == 0:
		verdict = VerdictInsufficient
	case report.LeakageDetected || report.CrossSymbolLeakage || !report.EpochIsolationPassed:
		verdict = VerdictBreach
	}

	report.Status = verdict
	report.Passed = passed(verdict)
	report.SummaryText = fmt.Sprintf(
		"Causality (replay of stored raws through production standardization and grid): %d past outputs compared; "+
			"%d changed when a perturbed future (%d measurements) followed. Cross-symbol: %d of the first symbol's outputs changed "+
			"when %d symbols were interleaved. Epoch: %d outputs differed between two fresh epochs. Verdict %s. "+
			"Signal-level causality (signals replayed from the raw tape) is not covered by this check.",
		report.ComparedObservations, report.ContaminatedCount, len(future),
		report.CrossSymbolContaminated, report.SymbolsTested, report.EpochContaminated, verdict,
	)

	return report
}

/*
AnalyzeCausality probes the production replay path on the audit sample, split
at the middle tick into past and future.
*/
func AnalyzeCausality(
	grid *store.Grid,
	ticks []int64,
	tickMeasurements map[int64][]*data.Measurement,
) CausalityAudit {
	if grid == nil || len(ticks) < 4 || len(tickMeasurements) == 0 {
		return CausalityAudit{
			SummaryText: "Insufficient ticks for causality and state isolation audit.",
			Status:      VerdictInsufficient,
		}
	}

	split := len(ticks) / 2
	var past, future []*data.Measurement

	for index, tick := range ticks {
		if index < split {
			past = append(past, tickMeasurements[tick]...)
			continue
		}

		future = append(future, tickMeasurements[tick]...)
	}

	return causalityProbe(productionReplay, grid, past, future, 1791)
}
