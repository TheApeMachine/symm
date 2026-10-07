package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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

	if opts.Symbol == "" {
		opts.Symbol = "BTC/USD"
	}

	if opts.MaxTicks <= 0 {
		opts.MaxTicks = 1000
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

	errnie.Info(fmt.Sprintf("[audit] starting audit for epoch %d on %s (max ticks: %d)", targetEpoch, opts.Symbol, opts.MaxTicks))

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
			fmt.Sprintf("[audit] zero measurements found for epoch %d on %s", targetEpoch, opts.Symbol),
			nil,
		))
	}

	errnie.Info(fmt.Sprintf(
		"[audit] successfully ingested %d ticks across %d raw series (%d canonical grid cells)",
		len(orderedTicks), len(rawSeries), len(canonicalSeries),
	))

	// Stage 0: Metric Contract Integrity
	contract := AnalyzeContract(allMeasurements)
	errnie.Info("[audit] Stage 0 completed: " + contract.SummaryText)

	// Stage 1: Metric Vitality & Redundancy
	vitality := AnalyzeVitality(orderedTicks, rawSeries, canonicalSeries)
	errnie.Info("[audit] Stage 1 completed: " + vitality.SummaryText)

	// Stage 2: Pair Sympathy & Permutation Null (evaluated on zero-centered deformations)
	sympathy := AnalyzeSympathy(orderedTicks, canonicalSeries, vitality.CanonicalCells, opts.Permutations)
	errnie.Info("[audit] Stage 2 completed: " + sympathy.SummaryText)

	// Stage 3: Grid Partitioning & Temporal Stability (cross-chronological split)
	stability := AnalyzeGridStability(orderedTicks, tickMeasurements, vitality.CanonicalCells, opts.Permutations)
	errnie.Info("[audit] Stage 3 completed: " + stability.SummaryText)

	// Stage 4: Token Dynamics on Unseen Data (train 60%, evaluate on held-out 40%)
	splitIdx := int(float64(len(orderedTicks)) * 0.60)
	if splitIdx < 20 {
		splitIdx = len(orderedTicks) / 2
	}
	trainTicks := orderedTicks[:splitIdx]
	unseenTicks := orderedTicks[splitIdx:]

	trainStream := store.NewStream()
	frozenGrid := store.NewGrid()
	feedGridFaithful(frozenGrid, trainStream, trainTicks, tickMeasurements)
	frozenGrid.Partition()
	frozenGrid.Settle()

	dynamics := AnalyzeTokenDynamics(frozenGrid, trainStream, unseenTicks, tickMeasurements, opts.Permutations)
	errnie.Info("[audit] Stage 4 completed: " + dynamics.SummaryText)

	// Stage 5: Precursor Informativeness (A->B Ignition and B->C Exhaustion)
	fullStream := store.NewStream()
	fullGrid := store.NewGrid()
	feedGridFaithful(fullGrid, fullStream, orderedTicks, tickMeasurements)
	fullGrid.Partition()
	fullGrid.Settle()

	precursor := AnalyzePrecursorSeparation(ctx, catalog, targetEpoch, opts.Symbol, fullGrid, orderedTicks, tickMeasurements, opts.Permutations)
	errnie.Info("[audit] Stage 5 completed: " + precursor.SummaryText)

	overallHealthy := contract.Passed && vitality.Passed && sympathy.Passed &&
		stability.Passed && dynamics.Passed && precursor.Passed

	report := &AuditReport{
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
		Epoch:          targetEpoch,
		Symbol:         opts.Symbol,
		TotalTicks:     len(orderedTicks),
		Contract:       contract,
		Vitality:       vitality,
		Sympathy:       sympathy,
		GridStability:  stability,
		TokenDynamics:  dynamics,
		Precursor:      precursor,
		OverallHealthy: overallHealthy,
	}

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
	filter := iceberg.NewAnd(
		iceberg.EqualTo(iceberg.Reference("epoch"), epoch),
		iceberg.EqualTo(iceberg.Reference("label"), symbol),
	)

	tasks, err := tbl.Scan(icetable.WithRowFilter(filter)).PlanFiles(s3Ctx)
	if err != nil {
		return nil, nil, nil, nil, nil, errnie.Error(errnie.Err(errnie.BadGateway, "[audit] failed to plan files", err))
	}

	if len(tasks) == 0 {
		return nil, nil, nil, nil, nil, nil
	}

	var allMeasurements []*data.Measurement
	seenTicks := make(map[int64]struct{})

	chunkSize := 40
	for taskIdx := 0; taskIdx < len(tasks); taskIdx += chunkSize {
		endIdx := taskIdx + chunkSize
		if endIdx > len(tasks) {
			endIdx = len(tasks)
		}

		scanOpts := []icetable.ScanOption{
			icetable.WithRowFilter(filter),
			icetable.WitMaxConcurrency(4),
		}

		_, batches, err := tbl.Scan(scanOpts...).ReadTasks(s3Ctx, tasks[taskIdx:endIdx])
		if err != nil {
			return nil, nil, nil, nil, nil, errnie.Error(errnie.Err(errnie.BadGateway, "[audit] failed to read tasks", err))
		}

		for batch, batchErr := range batches {
			if batchErr != nil {
				return nil, nil, nil, nil, nil, errnie.Error(errnie.Err(errnie.BadGateway, "[audit] batch decode failure in measurements", batchErr))
			}

			if batch != nil {
				measurements, readErr := tables.ReadMeasurements(batch)
				batch.Release()

				if readErr != nil {
					return nil, nil, nil, nil, nil, errnie.Error(errnie.Err(errnie.BadGateway, "[audit] failed to decode measurement batch", readErr))
				}

				for _, m := range measurements {
					if m != nil {
						allMeasurements = append(allMeasurements, m)
						seenTicks[m.Tick] = struct{}{}
					}
				}
			}
		}

		if maxTicks > 0 && len(seenTicks) >= maxTicks+50 {
			break
		}
	}

	if len(allMeasurements) == 0 {
		return nil, nil, nil, nil, nil, nil
	}

	// Stable chronological sort by SeqIdx and Tick
	sort.SliceStable(allMeasurements, func(i, j int) bool {
		if allMeasurements[i].SeqIdx != allMeasurements[j].SeqIdx {
			return allMeasurements[i].SeqIdx < allMeasurements[j].SeqIdx
		}
		return allMeasurements[i].Tick < allMeasurements[j].Tick
	})

	// Group measurements by tick preserving chronological tick order
	tickMeasurements := make(map[int64][]*data.Measurement)
	var tickOrder []int64
	seenTicks = make(map[int64]struct{})

	for _, meas := range allMeasurements {
		if meas == nil {
			continue
		}

		tick := meas.Tick
		if _, ok := seenTicks[tick]; !ok {
			seenTicks[tick] = struct{}{}
			tickOrder = append(tickOrder, tick)
		}

		tickMeasurements[tick] = append(tickMeasurements[tick], meas)
	}

	if maxTicks > 0 && len(tickOrder) > maxTicks {
		tickOrder = tickOrder[:maxTicks]
	}

	// Filter allMeasurements to the retained tick window
	retainedTickSet := make(map[int64]struct{}, len(tickOrder))
	for _, t := range tickOrder {
		retainedTickSet[t] = struct{}{}
	}

	filteredMeasurements := make([]*data.Measurement, 0, len(allMeasurements))
	for _, m := range allMeasurements {
		if _, ok := retainedTickSet[m.Tick]; ok {
			filteredMeasurements = append(filteredMeasurements, m)
		}
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
			if canonicalSeries[cellKey] == nil {
				canonicalSeries[cellKey] = make(map[int64]float64)
			}
			canonicalSeries[cellKey][tick] = val
		}
	}

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
