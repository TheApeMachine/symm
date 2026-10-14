package audit

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/store"
	s3store "github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/strategy"
)

/*
AuditOptions holds configurable execution parameters for the audit command.
*/
type AuditOptions struct {
	Epoch        int64
	Symbol       string
	MaxTicks     int
	Permutations int
	OutputDir    string
	NoPlots      bool
	TakerFee     float64
	// Thresholds are the named decision parameters; the zero value means
	// DefaultThresholds.
	Thresholds Thresholds
}

/*
Run executes the complete 6-stage health audit (Stage 0 to Stage 5) and generates
machine-readable and visual outputs in the specified directory.
*/
func Run(ctx context.Context, client *s3store.Client, opts AuditOptions) (*AuditReport, error) {
	if client == nil {
		return nil, errnie.Error(errnie.Err(errnie.Validation, "[audit] client is required", nil))
	}

	if opts.MaxTicks < 0 {
		opts.MaxTicks = 0
	}

	if opts.Permutations <= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"[audit] permutations is required and must be strictly positive",
			nil,
		))
	}

	if opts.TakerFee <= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"[audit] taker fee is required and must be strictly positive (specify via --taker-fee with explicit provenance)",
			nil,
		))
	}

	if opts.OutputDir == "" {
		opts.OutputDir = "audit_results"
	}

	if opts.Thresholds == (Thresholds{}) {
		opts.Thresholds = DefaultThresholds()
	}

	alpha := opts.Thresholds.Significance

	if err := os.MkdirAll(opts.OutputDir, 0755); err != nil {
		return nil, errnie.Error(errnie.Err(errnie.IO, "[audit] failed to create output directory", err))
	}

	// An explicit epoch is audited as given. Otherwise the newest run that
	// has stored measurements is chosen: a run that just started (or whose
	// commits never landed) has none, and auditing it says nothing.
	candidates := []int64{opts.Epoch}

	if opts.Epoch <= 0 {
		seen := make(map[int64]bool)

		for measurement := range client.Get("") {
			if measurement == nil || measurement.Epoch <= 0 || seen[measurement.Epoch] {
				continue
			}

			seen[measurement.Epoch] = true
			candidates = append(candidates, measurement.Epoch)
		}

		if len(candidates) == 0 {
			return nil, errnie.Error(errnie.Err(errnie.NotFound, "[audit] no recorded runs found to audit", nil))
		}

		slices.Sort(candidates)
		slices.Reverse(candidates)
	}

	targetEpoch := candidates[0]
	ticksDesc := fmt.Sprintf("%d", opts.MaxTicks)

	if opts.MaxTicks == 0 {
		ticksDesc = "all available ticks (unlimited)"
	}

	targetSymbol := opts.Symbol
	symbolDesc := targetSymbol
	if symbolDesc == "" {
		symbolDesc = "ALL_MARKET_SYMBOLS"
	}

	auditStart := time.Now()
	errnie.Info(fmt.Sprintf("[audit] starting audit on %s (ticks: %s)", symbolDesc, ticksDesc))

	// 1. Ingest observations from store with chronological ordering and strict error propagation
	var (
		orderedTicks     []int64
		tickMeasurements map[int64][]*data.Measurement
		rawSeries        map[string]map[int64]float64
		canonicalSeries  map[string]map[int64]float64
		allMeasurements  []*data.Measurement
		err              error
	)

	for _, epoch := range candidates {
		targetEpoch = epoch
		orderedTicks, tickMeasurements, rawSeries, canonicalSeries, allMeasurements, err = ingestMetrics(
			client, targetEpoch, opts.Symbol, opts.MaxTicks,
		)

		if err != nil {
			return nil, errnie.Error(err)
		}

		if len(orderedTicks) > 0 {
			break
		}

		if len(candidates) > 1 {
			errnie.Warn(fmt.Sprintf("[audit] run %d has no stored measurements; trying the previous run", epoch))
		}
	}

	if len(candidates) > 1 && len(orderedTicks) > 0 {
		errnie.Info(fmt.Sprintf("[audit] auditing run %d: the newest run with stored measurements", targetEpoch))
	}

	if len(orderedTicks) == 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.NotFound,
			fmt.Sprintf("[audit] zero measurements found for epoch %d on %s", targetEpoch, symbolDesc),
			nil,
		))
	}

	// Stage 0: Metric Contract Integrity
	stage0Start := time.Now()
	errnie.Info("[audit] [2/7 Stage 0] Evaluating Metric Contract Integrity...")
	contract := AnalyzeContract(allMeasurements, alpha)
	errnie.Info(fmt.Sprintf("[audit] [2/7 Stage 0] Completed in %s: %s", time.Since(stage0Start).Round(time.Millisecond), contract.SummaryText))

	// Stage 0.5: Microstructure Timing & Synchronization
	stage05Start := time.Now()
	errnie.Info("[audit] [2.5/7 Stage 0.5] Evaluating Microstructure Timing & Ingress Sequencing...")
	timing := AnalyzeTiming(allMeasurements, opts.Thresholds)
	errnie.Info(fmt.Sprintf("[audit] [2.5/7 Stage 0.5] Completed in %s: %s", time.Since(stage05Start).Round(time.Millisecond), timing.SummaryText))

	// Stage 1: Metric Vitality & Redundancy
	stage1Start := time.Now()
	errnie.Info(fmt.Sprintf("[audit] [3/7 Stage 1] Evaluating Metric Vitality & Subspace Redundancy across %d series...", len(rawSeries)))
	vitality := AnalyzeVitality(orderedTicks, rawSeries, canonicalSeries)
	errnie.Info(fmt.Sprintf("[audit] [3/7 Stage 1] Completed in %s: %s", time.Since(stage1Start).Round(time.Millisecond), vitality.SummaryText))

	// Stage 2: Pair Sympathy & Permutation Null (evaluated on zero-centered deformations)
	stage2Start := time.Now()
	errnie.Info(fmt.Sprintf("[audit] [4/7 Stage 2] Evaluating Pair Sympathy vs %d-iteration Permutation Null...", opts.Permutations))
	sympathy := AnalyzeSympathy(orderedTicks, canonicalSeries, vitality.CanonicalCells, opts.Permutations, alpha)
	errnie.Info(fmt.Sprintf("[audit] [4/7 Stage 2] Completed in %s: %s", time.Since(stage2Start).Round(time.Millisecond), sympathy.SummaryText))

	// Stage 3: Grid Partitioning, Temporal Stability & Completeness
	stage3Start := time.Now()
	errnie.Info(fmt.Sprintf("[audit] [5/7 Stage 3] Developing Disjoint Grids & Measuring Temporal Partition Stability across %d ticks...", len(orderedTicks)))
	stability := AnalyzeGridStability(store.NewGrid(ctx), orderedTicks, tickMeasurements, opts.Permutations, alpha)
	completeness := AnalyzeGridCompleteness(store.NewGrid(ctx), orderedTicks, tickMeasurements)
	stability.Completeness = completeness
	errnie.Info(fmt.Sprintf("[audit] [5/7 Stage 3] Completed in %s: %s | %s", time.Since(stage3Start).Round(time.Millisecond), stability.SummaryText, completeness.SummaryText))

	// Stage 4: Token Dynamics on Unseen Data (train 60%, evaluate on held-out 40%)
	stage4Start := time.Now()
	splitIdx := int(float64(len(orderedTicks)) * 0.60)
	if splitIdx < 20 {
		splitIdx = len(orderedTicks) / 2
	}
	unseenTicks := orderedTicks[splitIdx:]

	errnie.Info(fmt.Sprintf("[audit] [6/7 Stage 4] Replaying Held-Out Tape (%d unseen ticks): Token Dynamics & Region Excitation...", len(unseenTicks)))
	frozenGrid := store.NewGrid(ctx)

	dynamics := AnalyzeTokenDynamics(frozenGrid, unseenTicks, tickMeasurements, opts.Permutations, alpha)
	errnie.Info(fmt.Sprintf("[audit] [6/7 Stage 4] Completed in %s: %s", time.Since(stage4Start).Round(time.Millisecond), dynamics.SummaryText))

	// Stage 5: Precursor Informativeness (A->B Ignition and B->C Exhaustion)
	stage5Start := time.Now()
	errnie.Info("[audit] [7/7 Stage 5] Evaluating Precursor Divergence (Ignition & Exhaustion Separation)...")
	fullGrid := store.NewGrid(ctx)

	var minTick, maxTick int64
	if len(orderedTicks) > 0 {
		minTick = orderedTicks[0]
		maxTick = orderedTicks[len(orderedTicks)-1]
	}

	detections, detErr := loadOrDetectExcursions(client, targetEpoch, opts.Symbol, opts.TakerFee, minTick, maxTick)

	if detErr != nil {
		errnie.Warn("[audit] excursion detection retrieval: " + detErr.Error())
	}

	precursor := AnalyzePrecursorSeparation(
		ctx, client, targetEpoch, opts.Symbol, fullGrid, orderedTicks, tickMeasurements, opts.Permutations, alpha, detections, opts.TakerFee,
	)
	// Hypothesis tests on detections made at another fee than declared test
	// other excursions than the report claims.
	precursor.FeeProvenance = checkFeeProvenance(detections, opts.TakerFee)

	if !precursor.FeeProvenance.Consistent && len(detections) > 0 {
		for _, status := range []*string{
			&precursor.Status, &precursor.IgnitionHypothesis.Status,
			&precursor.ExhaustionHypothesis.Status, &precursor.PredictiveSkill.Status,
		} {
			*status = VerdictInvalid
		}

		precursor.Passed = false
		precursor.IgnitionHypothesis.Passed = false
		precursor.ExhaustionHypothesis.Passed = false
		precursor.PredictiveSkill.Passed = false
	}

	precursor.SummaryText += " Fee provenance: " + precursor.FeeProvenance.SummaryText + "."
	errnie.Info(fmt.Sprintf("[audit] [7/7 Stage 5] Completed in %s: %s", time.Since(stage5Start).Round(time.Millisecond), precursor.SummaryText))

	// Stage 6: Cognitive Engine & Radix Trie Learning Dynamics
	stage6Start := time.Now()
	errnie.Info(fmt.Sprintf("[audit] [Bonus Stage 6] Auditing Cognitive Engine & Radix Trie Dynamics (%d detections, %d permutations)...", len(detections), opts.Permutations))
	cognitive := AnalyzeCognitiveTrie(
		ctx, client, targetEpoch, opts.Symbol, fullGrid, orderedTicks, tickMeasurements, opts.Permutations, alpha, detections, opts.TakerFee,
	)
	errnie.Info(fmt.Sprintf("[audit] [Bonus Stage 6] Completed in %s: %s", time.Since(stage6Start).Round(time.Millisecond), cognitive.SummaryText))

	// Excursion Dynamic Transitions
	traces := ExtractExcursionTraces(ctx, client, targetEpoch, fullGrid, detections, tickMeasurements, 5)
	precursor.ExcursionTraces = traces


	// Validation 1: Production-vs-Audit Equivalence
	eqStart := time.Now()
	errnie.Info("[audit] [Validation 1/4] Auditing Production-vs-Audit Equivalence...")
	equivalence := AnalyzeEquivalence(ctx, fullGrid, orderedTicks, tickMeasurements)
	errnie.Info(fmt.Sprintf("[audit] [Validation 1/4] Completed in %s: %s", time.Since(eqStart).Round(time.Millisecond), equivalence.SummaryText))

	// Validation 2: Metric Truthfulness
	truthStart := time.Now()
	errnie.Info("[audit] [Validation 2/4] Auditing Metric Truthfulness...")
	// The tick sample is whatever data files were read first, so it holds a
	// scattered subset of cvd frames. Truthfulness needs complete streams:
	// both the trade tape and cvd are read in full from the epoch start to
	// the sample's last tick for every sampled symbol.
	trades, tradeErr := loadSampleStream(client, targetEpoch, "spot:trade", orderedTicks, allMeasurements)

	if tradeErr != nil {
		return nil, errnie.Error(tradeErr)
	}

	cvdFrames, cvdErr := loadSampleStream(client, targetEpoch, "cvd", orderedTicks, allMeasurements)

	if cvdErr != nil {
		return nil, errnie.Error(cvdErr)
	}

	// The level3 tape from the epoch start rebuilds the book every trade was
	// stamped from, beginning at the subscription snapshot.
	level3, level3Err := loadSampleStream(client, targetEpoch, "spot:level3", orderedTicks, allMeasurements)

	if level3Err != nil {
		return nil, errnie.Error(level3Err)
	}

	truthfulness := AnalyzeTruthfulness(ctx, trades, level3, cvdFrames)
	errnie.Info(fmt.Sprintf("[audit] [Validation 2/4] Completed in %s: %s", time.Since(truthStart).Round(time.Millisecond), truthfulness.SummaryText))

	// Validation 3: Causality & State Isolation
	causStart := time.Now()
	errnie.Info("[audit] [Validation 3/4] Auditing Causality & State Isolation...")
	causality := AnalyzeCausality(fullGrid, orderedTicks, tickMeasurements)
	errnie.Info(fmt.Sprintf("[audit] [Validation 3/4] Completed in %s: %s", time.Since(causStart).Round(time.Millisecond), causality.SummaryText))

	// Validation 4: Grid Sensitivity & Family Dependence
	sensStart := time.Now()
	errnie.Info("[audit] [Validation 4/4] Auditing Grid Sensitivity & Family Dependence...")
	sensitivity := AnalyzeGridSensitivity(fullGrid, orderedTicks, tickMeasurements, opts.Thresholds)
	errnie.Info(fmt.Sprintf("[audit] [Validation 4/4] Completed in %s: %s", time.Since(sensStart).Round(time.Millisecond), sensitivity.SummaryText))

	reportSymbol := opts.Symbol
	if reportSymbol == "" {
		reportSymbol = "ALL"
	}

	report := &AuditReport{
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
		Epoch:         targetEpoch,
		Symbol:        reportSymbol,
		TotalTicks:    len(orderedTicks),
		Contract:      contract,
		Timing:        timing,
		Vitality:      vitality,
		Sympathy:         sympathy,
		GridStability:    stability,
		GridCompleteness: completeness,
		TokenDynamics:    dynamics,
		Precursor:        precursor,
		CognitiveTrie:    cognitive,
		ExcursionTraces:  traces,
		Equivalence:      &equivalence,
		Truthfulness:     &truthfulness,
		Causality:        &causality,
		Sensitivity:      &sensitivity,
		Thresholds:       opts.Thresholds,
	}

	report.Verdicts = BuildVerdicts(report)
	report.Overall = OverallState(report.Verdicts)
	report.OverallHealthy = report.Overall == "ALL TESTS PASSED"

	errnie.Info(fmt.Sprintf("[audit] All stages computed in %s. Saving artifacts...", time.Since(auditStart).Round(time.Second)))

	// 2. Write machine-readable JSON
	jsonPath := filepath.Join(opts.OutputDir, "audit_results.json")
	jsonData, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return nil, errnie.Error(errnie.Err(errnie.Internal, "[audit] failed to marshal audit report", err))
	}

	if err := os.WriteFile(jsonPath, jsonData, 0644); err != nil {
		return nil, errnie.Error(errnie.Err(errnie.IO, "[audit] failed to write audit_results.json", err))
	}

	errnie.Info("[audit] machine-readable results saved to: " + jsonPath)

	// 3. Render Visualizations
	if !opts.NoPlots {
		if err := renderVisualizations(jsonPath, opts.OutputDir); err != nil {
			errnie.Warn("[audit] visualization script encountered error: " + err.Error())
		}
	}

	// 4. Generate AUDIT_SUMMARY.md
	summaryPath := filepath.Join(opts.OutputDir, "AUDIT_SUMMARY.md")
	summaryMd := GenerateSummaryMarkdown(report)
	report.SummaryMarkdown = summaryMd

	if err := os.WriteFile(summaryPath, []byte(summaryMd), 0644); err != nil {
		errnie.Warn("[audit] failed to write AUDIT_SUMMARY.md: " + err.Error())
		return report, nil
	}

	errnie.Info("[audit] executive summary document saved to: " + summaryPath)
	return report, nil
}

/*
ingestMetrics retrieves measurements from Iceberg, sorts them globally by chronological order,
and extracts both raw producer series and canonical grid input series.
Fails loudly on any scan or decode failure.
*/
func ingestMetrics(
	client *s3store.Client,
	epoch int64,
	symbol string,
	maxTicks int,
) (
	[]int64,
	map[int64][]*data.Measurement,
	map[string]map[int64]float64,
	map[string]map[int64]float64,
	[]*data.Measurement,
	error,
) {
	prefix := fmt.Sprintf("%d/", epoch)
	if symbol != "" {
		prefix = fmt.Sprintf("%d/%s/", epoch, symbol)
	}

	tickMeasurements := make(map[int64][]*data.Measurement)
	seenTicks := make(map[int64]struct{})
	ingestStart := time.Now()

	for measurement := range client.Get(prefix) {
		if measurement == nil {
			continue
		}

		if symbol != "" && measurement.Label != symbol {
			continue
		}

		if !slices.Contains(strategy.SensorySources, measurement.Source) {
			continue
		}

		if maxTicks > 0 && measurement.Tick > int64(maxTicks) {
			continue
		}

		tickMeasurements[measurement.Tick] = append(tickMeasurements[measurement.Tick], measurement)
		seenTicks[measurement.Tick] = struct{}{}
	}

	if len(seenTicks) == 0 {
		return nil, nil, nil, nil, nil, nil
	}

	tickOrder := make([]int64, 0, len(seenTicks))
	for tick := range seenTicks {
		tickOrder = append(tickOrder, tick)
	}

	slices.Sort(tickOrder)

	if maxTicks > 0 && len(tickOrder) > maxTicks {
		prunedTicks := tickOrder[maxTicks:]
		tickOrder = tickOrder[:maxTicks]

		for _, prunedTick := range prunedTicks {
			delete(tickMeasurements, prunedTick)
		}
	}

	// Count occurrences of each symbol across ticks to determine which symbols
	// were actually seen with sufficient empirical support (>= 2 observations for SNM transition).
	symbolTickCounts := make(map[string]int)
	for _, tick := range tickOrder {
		seenInTick := make(map[string]struct{})
		for _, meas := range tickMeasurements[tick] {
			if meas != nil && meas.Label != "" {
				seenInTick[meas.Label] = struct{}{}
			}
		}
		for sym := range seenInTick {
			symbolTickCounts[sym]++
		}
	}

	activeSymbols := make(map[string]struct{})
	for sym, count := range symbolTickCounts {
		if count >= 2 || (symbol != "" && sym == symbol) {
			activeSymbols[sym] = struct{}{}
		}
	}

	if len(activeSymbols) == 0 {
		for sym := range symbolTickCounts {
			activeSymbols[sym] = struct{}{}
		}
	}

	prunedTickMeasurements := make(map[int64][]*data.Measurement)
	activeTickOrder := make([]int64, 0, len(tickOrder))
	totalRetained := 0

	for _, tick := range tickOrder {
		var activeGroup []*data.Measurement
		for _, meas := range tickMeasurements[tick] {
			if meas == nil {
				errnie.Error(errnie.Err(
					errnie.Validation,
					"[audit] nil measurement in tick measurements",
					nil,
				), "tick", tick)

				continue
			}

			if _, ok := activeSymbols[meas.Label]; ok || meas.Label == "" {
				activeGroup = append(activeGroup, meas)
			}
		}

		if len(activeGroup) > 0 {
			slices.SortFunc(activeGroup, func(left, right *data.Measurement) int {
				return cmp.Compare(left.SeqIdx, right.SeqIdx)
			})
			prunedTickMeasurements[tick] = activeGroup
			activeTickOrder = append(activeTickOrder, tick)
			totalRetained += len(activeGroup)
		}
	}

	tickMeasurements = prunedTickMeasurements
	tickOrder = activeTickOrder

	filteredMeasurements := make([]*data.Measurement, 0, totalRetained)
	for _, tick := range tickOrder {
		filteredMeasurements = append(filteredMeasurements, tickMeasurements[tick]...)
	}

	// Extract raw producer named series
	rawSeries := make(map[string]map[int64]float64)
	for _, tick := range tickOrder {
		for _, meas := range tickMeasurements[tick] {
			for entry := range meas.Read() {
				if entry == nil || entry.Metric == nil {
					errnie.Error(errnie.Err(
						errnie.NotFound,
						"[audit] nil metric entry in measurement",
						nil,
					))

					continue
				}

				if entry.Metric.Label() == "checksum" {
					errnie.Error(errnie.Err(
						errnie.NotFound,
						"[audit] checksum metric in measurement",
						nil,
					))

					continue
				}

				name := seriesKey(meas.Source, meas.Label, entry.Metric.Label())

				if rawSeries[name] == nil {
					rawSeries[name] = make(map[int64]float64)
				}
				rawSeries[name][tick] = entry.Metric.Raw()
			}
		}
	}

	// Extract canonical grid cells directly from observed measurements
	canonicalSeries := make(map[string]map[int64]float64)
	for _, tick := range tickOrder {
		group := tickMeasurements[tick]

		if len(group) == 0 {
			errnie.Error(errnie.Err(
				errnie.NotFound,
				"[audit] group is empty",
				nil,
			))

			continue
		}

		for _, meas := range group {
			if meas == nil {
				errnie.Error(errnie.Err(
					errnie.NotFound,
					"[audit] nil measurement in audit slice",
					nil,
				))

				continue
			}

			for entry := range meas.Read() {
				if entry == nil || entry.Metric == nil {
					errnie.Error(errnie.Err(
						errnie.NotFound,
						"[audit] nil metric entry in measurement",
						nil,
					))

					continue
				}

				key := seriesKey(meas.Source, meas.Label, entry.Key)

				if canonicalSeries[key] == nil {
					canonicalSeries[key] = make(map[int64]float64)
				}
				canonicalSeries[key][tick] = entry.Metric.SNM()
			}
		}
	}

	errnie.Info(fmt.Sprintf(
		"[audit] [1/7 Ingest] Ingestion complete in %s: %d ticks across %d active symbols (%d raw series, %d canonical grid cells)",
		time.Since(ingestStart).Round(time.Second), len(tickOrder), len(activeSymbols), len(rawSeries), len(canonicalSeries),
	))

	return tickOrder, tickMeasurements, rawSeries, canonicalSeries, filteredMeasurements, nil
}

func renderVisualizations(jsonPath, outputDir string) error {
	scriptPath := filepath.Join("scripts", "plot_audit.py")
	if _, err := os.Stat(scriptPath); err != nil {
		return fmt.Errorf("plot script not found at %s: %w", scriptPath, err)
	}

	plotsDir := filepath.Join(outputDir, "plots")
	cmd := exec.Command("python3", scriptPath, "--input", jsonPath, "--output", plotsDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

/*
loadSampleStream reads one source's stored rows for every cvd symbol in the
sample from the start of the epoch through the sample's last tick, so each
row has its true predecessor. A read failure fails the audit.
*/
func loadSampleStream(
	client *s3store.Client,
	epoch int64,
	source string,
	ticks []int64,
	measurements []*data.Measurement,
) ([]*data.Measurement, error) {
	if len(ticks) == 0 {
		return nil, nil
	}

	symbolSet := make(map[string]struct{})

	for _, measurement := range measurements {
		if measurement != nil && measurement.Source == "cvd" {
			symbolSet[measurement.Label] = struct{}{}
		}
	}

	if len(symbolSet) == 0 {
		return nil, nil
	}

	symbols := make([]string, 0, len(symbolSet))

	for symbol := range symbolSet {
		symbols = append(symbols, symbol)
	}

	slices.Sort(symbols)

	var trades []*data.Measurement
	maxTick := ticks[len(ticks)-1]

	for _, symbol := range symbols {
		for trade := range client.Get(fmt.Sprintf("%d/%s/%s/", epoch, symbol, source)) {
			if trade == nil {
				continue
			}

			if trade.Tick > maxTick {
				continue
			}

			trades = append(trades, trade)
		}
	}

	return trades, nil
}
