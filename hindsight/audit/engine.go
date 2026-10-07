package audit

import (
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
	"github.com/theapemachine/symm/nomagique/store"
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
Run executes the complete 5-stage health audit and generates machine-readable
and visual outputs in the specified directory.
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

	// 1. Ingest observations from Iceberg
	ticks, metricSeries, err := ingestMetrics(ctx, catalog, targetEpoch, opts.Symbol, opts.MaxTicks)
	if err != nil {
		return nil, errnie.Error(err)
	}

	if len(ticks) == 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.NotFound,
			fmt.Sprintf("[audit] zero measurements found for epoch %d on %s", targetEpoch, opts.Symbol),
			nil,
		))
	}

	errnie.Info(fmt.Sprintf("[audit] successfully ingested %d ticks across %d distinct metrics", len(ticks), len(metricSeries)))

	// Stage 1: Metric Vitality & Redundancy
	vitality := AnalyzeVitality(ticks, metricSeries)
	errnie.Info("[audit] Stage 1 completed: " + vitality.SummaryText)

	// Stage 2: Pair Sympathy & Permutation Null
	sympathy := AnalyzeSympathy(ticks, metricSeries, vitality.Metrics, opts.Permutations)
	errnie.Info("[audit] Stage 2 completed: " + sympathy.SummaryText)

	// Stage 3: Grid Partitioning & Temporal Stability
	stability := AnalyzeGridStability(ticks, metricSeries, vitality.Metrics)
	errnie.Info("[audit] Stage 3 completed: " + stability.SummaryText)

	// Stage 4: Token Dynamics & Transition Structure
	fullGrid := store.NewGrid()
	feedGrid(fullGrid, ticks, metricSeries)
	fullGrid.Partition()

	dynamics := AnalyzeTokenDynamics(fullGrid, ticks, metricSeries)
	errnie.Info("[audit] Stage 4 completed: " + dynamics.SummaryText)

	// Stage 5: Precursor Informativeness
	precursor := AnalyzePrecursorSeparation(ctx, catalog, targetEpoch, opts.Symbol, fullGrid, ticks, metricSeries, opts.Permutations)
	errnie.Info("[audit] Stage 5 completed: " + precursor.SummaryText)

	overallHealthy := vitality.Passed && sympathy.Passed && stability.Passed && dynamics.Passed && precursor.Passed

	report := &AuditReport{
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
		Epoch:          targetEpoch,
		Symbol:         opts.Symbol,
		TotalTicks:     len(ticks),
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

	// 3. Render Python Visualizations
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

func ingestMetrics(
	ctx context.Context,
	catalog *tables.Catalog,
	epoch int64,
	symbol string,
	maxTicks int,
) ([]int64, map[string]map[int64]float64, error) {
	tbl, err := catalog.Load(ctx, tables.Measurements)
	if err != nil {
		return nil, nil, errnie.Error(errnie.Err(errnie.BadGateway, "[audit] failed to load measurements table", err))
	}

	s3Ctx := catalog.Context(ctx)
	filter := iceberg.NewAnd(
		iceberg.EqualTo(iceberg.Reference("epoch"), epoch),
		iceberg.EqualTo(iceberg.Reference("label"), symbol),
	)

	tasks, err := tbl.Scan(icetable.WithRowFilter(filter)).PlanFiles(s3Ctx)
	if err != nil {
		return nil, nil, errnie.Error(errnie.Err(errnie.BadGateway, "[audit] failed to plan files", err))
	}

	if len(tasks) == 0 {
		return nil, nil, nil
	}

	metricSeries := make(map[string]map[int64]float64)
	tickSet := make(map[int64]struct{})

	scanTasks := tasks

	_, batches, err := tbl.Scan(icetable.WithRowFilter(filter)).ReadTasks(s3Ctx, scanTasks)
	if err != nil {
		return nil, nil, errnie.Error(errnie.Err(errnie.BadGateway, "[audit] failed to read tasks", err))
	}

	for batch, batchErr := range batches {
		if batchErr != nil {
			break
		}

		measurements, readErr := tables.ReadMeasurements(batch)
		batch.Release()

		if readErr != nil {
			continue
		}

		for _, meas := range measurements {
			if meas == nil {
				continue
			}

			tick := meas.Tick
			tickSet[tick] = struct{}{}

			for entry := range meas.Read() {
				if entry == nil || entry.Metric == nil {
					continue
				}

				name := entry.Key
				if metricSeries[name] == nil {
					metricSeries[name] = make(map[int64]float64)
				}

				metricSeries[name][tick] = entry.Metric.Raw
			}
		}

		if len(tickSet) >= maxTicks {
			break
		}
	}

	orderedTicks := make([]int64, 0, len(tickSet))
	for tick := range tickSet {
		orderedTicks = append(orderedTicks, tick)
	}

	slices.Sort(orderedTicks)

	if len(orderedTicks) > maxTicks {
		orderedTicks = orderedTicks[:maxTicks]
	}

	return orderedTicks, metricSeries, nil
}

func renderVisualizations(jsonPath, outputDir string) error {
	scriptPath := filepath.Join("scripts", "plot_audit.py")
	if _, err := os.Stat(scriptPath); err != nil {
		return fmt.Errorf("script not found at %s", scriptPath)
	}

	cmd := exec.Command("python3", scriptPath, jsonPath, outputDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}
