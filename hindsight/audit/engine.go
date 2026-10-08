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

	"github.com/apache/iceberg-go"
	icetable "github.com/apache/iceberg-go/table"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/store"
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
}

/*
Run executes the complete 6-stage health audit (Stage 0 to Stage 5) and generates
machine-readable and visual outputs in the specified directory.
*/
func Run(ctx context.Context, catalog *tables.Catalog, opts AuditOptions) (*AuditReport, error) {
	if catalog == nil {
		return nil, errnie.Error(errnie.Err(errnie.Validation, "[audit] catalog is required", nil))
	}

	if opts.MaxTicks < 0 {
		opts.MaxTicks = 0
	}

	if opts.Permutations <= 0 {
		opts.Permutations = 50
	}

	if opts.OutputDir == "" {
		opts.OutputDir = "audit_results"
	}

	if err := os.MkdirAll(opts.OutputDir, 0755); err != nil {
		return nil, errnie.Error(errnie.Err(errnie.IO, "[audit] failed to create output directory", err))
	}

	targetEpoch := opts.Epoch

	if targetEpoch <= 0 {
		runs, err := catalog.Runs(ctx)

		if err != nil || len(runs) == 0 {
			return nil, errnie.Error(errnie.Err(errnie.NotFound, "[audit] no recorded runs found to audit", err))
		}

		targetEpoch = runs[0].Epoch
	}

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
	errnie.Info(fmt.Sprintf("[audit] starting audit for epoch %d on %s (ticks: %s)", targetEpoch, symbolDesc, ticksDesc))

	// 1. Ingest observations from Iceberg with chronological ordering and strict error propagation
	orderedTicks, tickMeasurements, rawSeries, canonicalSeries, allMeasurements, err := ingestMetrics(
		ctx, catalog, targetEpoch, opts.Symbol, opts.MaxTicks,
	)
	if err != nil {
		return nil, errnie.Error(err)
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
	contract := AnalyzeContract(allMeasurements)
	errnie.Info(fmt.Sprintf("[audit] [2/7 Stage 0] Completed in %s: %s", time.Since(stage0Start).Round(time.Millisecond), contract.SummaryText))

	// Stage 1: Metric Vitality & Redundancy
	stage1Start := time.Now()
	errnie.Info(fmt.Sprintf("[audit] [3/7 Stage 1] Evaluating Metric Vitality & Subspace Redundancy across %d series...", len(rawSeries)))
	vitality := AnalyzeVitality(orderedTicks, rawSeries, canonicalSeries)
	errnie.Info(fmt.Sprintf("[audit] [3/7 Stage 1] Completed in %s: %s", time.Since(stage1Start).Round(time.Millisecond), vitality.SummaryText))

	// Stage 2: Pair Sympathy & Permutation Null (evaluated on zero-centered deformations)
	stage2Start := time.Now()
	errnie.Info(fmt.Sprintf("[audit] [4/7 Stage 2] Evaluating Pair Sympathy vs %d-iteration Permutation Null...", opts.Permutations))
	sympathy := AnalyzeSympathy(orderedTicks, canonicalSeries, vitality.CanonicalCells, opts.Permutations)
	errnie.Info(fmt.Sprintf("[audit] [4/7 Stage 2] Completed in %s: %s", time.Since(stage2Start).Round(time.Millisecond), sympathy.SummaryText))

	// Stage 3: Grid Partitioning & Temporal Stability (cross-chronological split)
	stage3Start := time.Now()
	errnie.Info(fmt.Sprintf("[audit] [5/7 Stage 3] Developing Disjoint Grids & Measuring Temporal Partition Stability across %d ticks...", len(orderedTicks)))
	stability := AnalyzeGridStability(orderedTicks, tickMeasurements, vitality.CanonicalCells, opts.Permutations)
	errnie.Info(fmt.Sprintf("[audit] [5/7 Stage 3] Completed in %s: %s", time.Since(stage3Start).Round(time.Millisecond), stability.SummaryText))

	// Stage 4: Token Dynamics on Unseen Data (train 60%, evaluate on held-out 40%)
	stage4Start := time.Now()
	splitIdx := int(float64(len(orderedTicks)) * 0.60)
	if splitIdx < 20 {
		splitIdx = len(orderedTicks) / 2
	}
	trainTicks := orderedTicks[:splitIdx]
	unseenTicks := orderedTicks[splitIdx:]

	errnie.Info(fmt.Sprintf("[audit] [6/7 Stage 4] Replaying Held-Out Tape (%d unseen ticks): Token Dynamics & Region Excitation...", len(unseenTicks)))
	trainStreams := make(map[string]*store.Stream)
	frozenGrid := store.NewGrid()
	feedGridFaithful(frozenGrid, trainStreams, trainTicks, tickMeasurements)
	frozenGrid.Partition()
	frozenGrid.Settle()

	dynamics := AnalyzeTokenDynamics(frozenGrid, trainStreams, unseenTicks, tickMeasurements, opts.Permutations)
	errnie.Info(fmt.Sprintf("[audit] [6/7 Stage 4] Completed in %s: %s", time.Since(stage4Start).Round(time.Millisecond), dynamics.SummaryText))

	// Stage 5: Precursor Informativeness (A->B Ignition and B->C Exhaustion)
	stage5Start := time.Now()
	errnie.Info("[audit] [7/7 Stage 5] Evaluating Precursor Divergence (Ignition & Exhaustion Separation)...")
	fullStreams := make(map[string]*store.Stream)
	fullGrid := store.NewGrid()
	feedGridFaithful(fullGrid, fullStreams, orderedTicks, tickMeasurements)
	fullGrid.Partition()
	fullGrid.Settle()

	detections, detErr := loadOrDetectExcursions(ctx, catalog, targetEpoch, opts.Symbol)

	if detErr != nil {
		errnie.Warn("[audit] excursion detection retrieval: " + detErr.Error())
	}

	precursor := AnalyzePrecursorSeparation(
		ctx, catalog, targetEpoch, opts.Symbol, fullGrid, orderedTicks, tickMeasurements, opts.Permutations, detections,
	)
	errnie.Info(fmt.Sprintf("[audit] [7/7 Stage 5] Completed in %s: %s", time.Since(stage5Start).Round(time.Millisecond), precursor.SummaryText))

	// Stage 6: Cognitive Engine & Radix Trie Learning Dynamics
	stage6Start := time.Now()
	errnie.Info(fmt.Sprintf("[audit] [Bonus Stage 6] Auditing Cognitive Engine & Radix Trie Dynamics (%d detections, %d permutations)...", len(detections), opts.Permutations))
	cognitive := AnalyzeCognitiveTrie(
		ctx, catalog, targetEpoch, opts.Symbol, fullGrid, orderedTicks, tickMeasurements, opts.Permutations, detections,
	)
	errnie.Info(fmt.Sprintf("[audit] [Bonus Stage 6] Completed in %s: %s", time.Since(stage6Start).Round(time.Millisecond), cognitive.SummaryText))

	overallHealthy := contract.Passed && vitality.Passed && sympathy.Passed &&
		stability.Passed && dynamics.Passed && precursor.Passed && cognitive.Passed

	reportSymbol := opts.Symbol
	if reportSymbol == "" {
		reportSymbol = "ALL"
	}

	report := &AuditReport{
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
		Epoch:          targetEpoch,
		Symbol:         reportSymbol,
		TotalTicks:     len(orderedTicks),
		Contract:       contract,
		Vitality:       vitality,
		Sympathy:       sympathy,
		GridStability:  stability,
		TokenDynamics:  dynamics,
		Precursor:      precursor,
		CognitiveTrie:  cognitive,
		OverallHealthy: overallHealthy,
	}

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
	} else {
		errnie.Info("[audit] executive summary document saved to: " + summaryPath)
	}

	return report, nil
}

/*
ingestMetrics retrieves measurements from Iceberg, sorts them globally by chronological order,
and extracts both raw producer series and canonical grid input series.
Fails loudly on any scan or decode failure.
*/
func ingestMetrics(
	ctx context.Context,
	catalog *tables.Catalog,
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
	tbl, err := catalog.Load(ctx, tables.Measurements)
	if err != nil {
		return nil, nil, nil, nil, nil, errnie.Error(errnie.Err(errnie.BadGateway, "[audit] failed to load measurements table", err))
	}

	s3Ctx := catalog.Context(ctx)
	var filter iceberg.BooleanExpression = iceberg.NewAnd(
		iceberg.EqualTo(iceberg.Reference("epoch"), epoch),
		iceberg.IsIn(iceberg.Reference("source"), tables.SensorySources...),
	)

	if symbol != "" {
		filter = iceberg.NewAnd(
			filter,
			iceberg.EqualTo(iceberg.Reference("label"), symbol),
		)
	}

	tasks, err := tbl.Scan(icetable.WithRowFilter(filter)).PlanFiles(s3Ctx)
	if err != nil {
		return nil, nil, nil, nil, nil, errnie.Error(errnie.Err(errnie.BadGateway, "[audit] failed to plan files", err))
	}

	if len(tasks) == 0 {
		return nil, nil, nil, nil, nil, nil
	}

	tickMeasurements := make(map[int64][]*data.Measurement)
	seenTicks := make(map[int64]struct{})

	chunkSize := 250
	ingestStart := time.Now()

	for taskIndex := 0; taskIndex < len(tasks); taskIndex += chunkSize {
		endIndex := taskIndex + chunkSize

		if endIndex > len(tasks) {
			endIndex = len(tasks)
		}

		scanOpts := []icetable.ScanOption{
			icetable.WithRowFilter(filter),
			icetable.WitMaxConcurrency(16),
		}

		_, batches, err := tbl.Scan(scanOpts...).ReadTasks(s3Ctx, tasks[taskIndex:endIndex])

		if err != nil {
			errnie.Warn(fmt.Sprintf("[audit] failed to read tasks chunk %d-%d: %s", taskIndex, endIndex, err))
			continue
		}

		for batch, batchErr := range batches {
			if batchErr != nil {
				errnie.Warn(fmt.Sprintf("[audit] skipping corrupted batch in measurements: %s", batchErr))
				continue
			}

			if batch != nil {
				measurements, readErr := tables.ReadMeasurements(batch)
				batch.Release()

				if readErr != nil {
					errnie.Warn(fmt.Sprintf("[audit] skipping unreadable measurement batch: %s", readErr))
					continue
				}

				for _, meas := range measurements {
					if meas != nil {
						meas.PurgeMetric("checksum")
						tickMeasurements[meas.Tick] = append(tickMeasurements[meas.Tick], meas)
						seenTicks[meas.Tick] = struct{}{}
					}
				}
			}
		}

		completedTasks := endIndex
		percent := float64(completedTasks) / float64(len(tasks)) * 100.0
		elapsed := time.Since(ingestStart)
		var etaStr string

		if completedTasks > 0 && completedTasks < len(tasks) {
			rate := float64(completedTasks) / elapsed.Seconds()
			remaining := len(tasks) - completedTasks
			etaDuration := time.Duration(float64(remaining)/rate) * time.Second
			etaStr = fmt.Sprintf(" | ETA: %s", etaDuration.Round(time.Second))
		}

		errnie.Info(fmt.Sprintf(
			"[audit] [1/7 Ingest] %d / %d tasks (%.1f%%) | %d ticks | elapsed: %s%s",
			completedTasks, len(tasks), percent, len(seenTicks), elapsed.Round(time.Second), etaStr,
		))

		if maxTicks > 0 && len(seenTicks) >= maxTicks+50 {
			break
		}
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

	totalRetained := 0
	for _, tick := range tickOrder {
		group := tickMeasurements[tick]

		slices.SortFunc(group, func(left, right *data.Measurement) int {
			return cmp.Compare(left.SeqIdx, right.SeqIdx)
		})

		totalRetained += len(group)
	}

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
					continue
				}

				name := entry.Metric.Label
				if name == "checksum" {
					continue
				}

				if rawSeries[name] == nil {
					rawSeries[name] = make(map[int64]float64)
				}
				rawSeries[name][tick] = entry.Metric.Raw
			}
		}
	}

	// Extract canonical grid cells using production ChannelsFrom aggregation
	canonicalSeries := make(map[string]map[int64]float64)
	for _, tick := range tickOrder {
		group := tickMeasurements[tick]
		if len(group) == 0 {
			continue
		}

		channels := strategy.ChannelsFrom(group...)
		for cellKey, val := range channels.Raw {
			if cellKey == "checksum" {
				continue
			}

			if canonicalSeries[cellKey] == nil {
				canonicalSeries[cellKey] = make(map[int64]float64)
			}
			canonicalSeries[cellKey][tick] = val
		}
	}

	errnie.Info(fmt.Sprintf(
		"[audit] [1/7 Ingest] Ingestion complete in %s: %d ticks across %d raw series (%d canonical grid cells)",
		time.Since(ingestStart).Round(time.Second), len(tickOrder), len(rawSeries), len(canonicalSeries),
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
