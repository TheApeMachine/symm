package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/nomagique/runtime"
	s3store "github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
	"golang.org/x/sync/errgroup"
)

type AuditorStage interface {
	Channel() chan *data.Value
	Run() error
	Report() string
	Name() string
	Data() any
}

type Violation struct {
	Symbol      string
	Name        string
	Description string
	Measurement *data.Value
	Metric      any
}

func NewViolation(
	symbol, name, description string,
	measurement *data.Value,
	metric any) *Violation {
	return &Violation{
		Symbol:      symbol,
		Name:        name,
		Description: description,
		Measurement: measurement,
		Metric:      metric,
	}
}

func (violation *Violation) String() string {
	tmpl := `
- [VIOLATION] {{.Name}},
  **SYMBOL:** {{.Symbol}},
  **DESCRIPTION:** {{.Description}},
  **METRIC:** {{.Metric.Name}},
`

	t := template.Must(template.New("violation").Parse(tmpl))

	var b strings.Builder

	if err := t.Execute(&b, violation); err != nil {
		errnie.Error(errnie.Err(errnie.UnprocessableContent, "[audit] violation render failed for "+violation.Name, err))
	}

	return b.String()
}

/*
Engine is the composition root and orchestrator for the empirical audit.
It loads and validates datasets, coordinates stage evaluations concurrently,
and produces the synthesized audit report and artifacts.
*/
type Engine struct {
	*runtime.System
	store  *s3store.Client
	stages []AuditorStage
	epochs []int64
}

/*
NewEngine constructs an audit engine using system.Cfg.Audit defaults.
*/
func NewEngine(ctx context.Context) *Engine {
	store := s3store.NewClient(ctx, "symm")
	epochs := make([]int64, 0, 1)

	return &Engine{
		System: runtime.NewSystem(ctx, "audit:engine"),
		store:  store,
		stages: []AuditorStage{
			NewTimingStage(ctx),
			NewContractStage(ctx),
			NewVitalityStage(ctx),
			NewRedundancyStage(ctx),
			NewSympathyStage(ctx),
			NewStabilityStage(ctx),
			NewDynamicsStage(ctx),
			NewPrecursorStage(ctx),
			NewCognitiveStage(ctx),
			NewTruthfulnessStage(ctx),
		},
		epochs: epochs,
	}
}

/*
resolveTargetEpoch determines the single epoch to audit.
If system.Cfg.Audit.Epoch is explicitly configured (> 0), it is used directly.
Otherwise, it discovers all completed epochs under run/epoch/ and picks the newest
epoch that has signal measurements stored in S3.
*/
func (engine *Engine) resolveTargetEpoch(symbol string) (int64, error) {
	if system.Cfg.Audit.Epoch > 0 {
		return system.Cfg.Audit.Epoch, nil
	}

	if len(engine.epochs) > 0 && engine.epochs[0] > 0 {
		return engine.epochs[0], nil
	}

	seen := make(map[int64]bool)
	var candidateEpochs []int64

	for page := range engine.store.List("run/epoch/", "/") {
		for _, commonPrefix := range page.CommonPrefixes {
			if commonPrefix.Prefix == nil {
				continue
			}

			prefixStr := strings.TrimPrefix(*commonPrefix.Prefix, "run/epoch/")
			prefixStr = strings.Trim(prefixStr, "/")
			epoch, err := strconv.ParseInt(prefixStr, 10, 64)

			if err != nil || epoch <= 0 {
				continue
			}

			if seen[epoch] {
				continue
			}

			seen[epoch] = true
			candidateEpochs = append(candidateEpochs, epoch)
		}

		for _, object := range page.Contents {
			if object.Key == nil {
				continue
			}

			parts := strings.Split(*object.Key, "/")

			if len(parts) < 3 {
				continue
			}

			epoch, err := strconv.ParseInt(parts[2], 10, 64)

			if err != nil || epoch <= 0 {
				continue
			}

			if seen[epoch] {
				continue
			}

			seen[epoch] = true
			candidateEpochs = append(candidateEpochs, epoch)
		}
	}

	if len(candidateEpochs) == 0 {
		return 0, errnie.Error(errnie.Err(
			errnie.NotFound,
			"[audit:engine] no completed epochs found in store under run/epoch/",
			nil,
		))
	}

	slices.SortFunc(candidateEpochs, func(firstEpoch, secondEpoch int64) int {
		if firstEpoch > secondEpoch {
			return -1
		}

		if firstEpoch < secondEpoch {
			return 1
		}

		return 0
	})

	for _, candidate := range candidateEpochs {
		testPrefix := fmt.Sprintf("%s/%s/%d", types.SignalSources[0], symbol, candidate)
		keys := engine.store.ListKeys(testPrefix)

		if len(keys) > 0 {
			return candidate, nil
		}
	}

	return candidateEpochs[0], nil
}

/*
resolveSymbols discovers all active symbols with recorded signals in targetEpoch.
If system.Cfg.Audit.Symbol is explicitly configured, it is prioritized.
*/
func (engine *Engine) resolveSymbols(targetEpoch int64, defaultSymbol string) []string {
	if system.Cfg.Audit.Symbol != "" {
		return []string{system.Cfg.Audit.Symbol}
	}

	seen := make(map[string]bool)
	symbols := make([]string, 0)

	for _, key := range engine.store.ListKeys(string(types.SignalSources[0]) + "/") {
		parts := strings.Split(key, "/")

		if len(parts) < 3 {
			continue
		}

		epochIdx := -1
		var epoch int64
		for i := 1; i < len(parts); i++ {
			parsed, err := strconv.ParseInt(parts[i], 10, 64)
			if err == nil && parsed > 0 {
				epochIdx = i
				epoch = parsed
				break
			}
		}

		if epochIdx < 2 || epoch != targetEpoch {
			continue
		}

		sym := strings.Join(parts[1:epochIdx], "/")

		if !seen[sym] {
			seen[sym] = true
			symbols = append(symbols, sym)
		}
	}

	if len(symbols) == 0 {
		return []string{defaultSymbol}
	}

	slices.Sort(symbols)
	return symbols
}

/*
Run executes the complete 6-stage health audit and generates machine-readable
and visual outputs.
*/
func (engine *Engine) Run() error {
	group, _ := errgroup.WithContext(engine.Context())

	for _, stage := range engine.stages {
		group.Go(func() error {
			return stage.Run()
		})
	}

	group.Go(func() error {
		symbol := system.Cfg.Audit.Symbol

		if symbol == "" {
			symbol = "BTC/USD"
		}

		targetEpoch, err := engine.resolveTargetEpoch(symbol)

		if err != nil {
			return errnie.Error(err)
		}

		symbols := engine.resolveSymbols(targetEpoch, symbol)
		errnie.Info(fmt.Sprintf("[audit:engine] auditing isolated epoch %d for symbols: %v", targetEpoch, symbols))

		var signalStages []AuditorStage
		var precursorStages []AuditorStage
		var contractStages []AuditorStage
		var cognitiveStages []AuditorStage

		for _, stage := range engine.stages {
			switch stage.Name() {
			case "audit:precursor":
				precursorStages = append(precursorStages, stage)
			case "audit:contract":
				contractStages = append(contractStages, stage)
			case "audit:cognitive":
				cognitiveStages = append(cognitiveStages, stage)
			default:
				signalStages = append(signalStages, stage)
			}
		}

		var signalPrefixes []string

		for _, sym := range symbols {
			for _, source := range types.SignalSources {
				signalPrefixes = append(signalPrefixes, fmt.Sprintf("%s/%s/%019d", source, sym, targetEpoch))
				if fmt.Sprintf("%d", targetEpoch) != fmt.Sprintf("%019d", targetEpoch) {
					signalPrefixes = append(signalPrefixes, fmt.Sprintf("%s/%s/%d", source, sym, targetEpoch))
				}
			}
		}

		var currentTick uint64
		var hasTick bool
		var currentPeers []*data.Value
		var uniqueTicksCount int

		flushJoin := func() {
			if len(currentPeers) == 0 {
				return
			}

			first := currentPeers[0]
			join := data.NewValue(first.Origin, "runtime:join", first.Scope)
			join.Tick = first.Tick
			join.SeqIdx = first.SeqIdx
			join.Timestamp = first.Timestamp
			for _, peer := range currentPeers {
				join.Layers = append(join.Layers, peer.Layers...)
				join.Index = append(join.Index, peer.Index...)
			}

			for _, stage := range signalStages {
				stage.Channel() <- join
			}

			for _, stage := range precursorStages {
				stage.Channel() <- join
			}

			for _, stage := range contractStages {
				stage.Channel() <- join
			}

			currentPeers = nil
		}

		for measurement := range hindsight.Measurements(engine.store, signalPrefixes...) {
			if hasTick && measurement.Tick != currentTick {
				flushJoin()
				uniqueTicksCount++

				if system.Cfg.Audit.Ticks > 0 && uniqueTicksCount >= system.Cfg.Audit.Ticks {
					break
				}
			}

			hasTick = true
			currentTick = measurement.Tick
			currentPeers = append(currentPeers, measurement)

			for _, stage := range signalStages {
				stage.Channel() <- measurement
			}

			for _, stage := range contractStages {
				stage.Channel() <- measurement
			}
		}

		flushJoin()

		// Stream detector excursions exclusively to PrecursorStage and ContractStage
		for _, sym := range symbols {
			detectorPrefixes := []string{
				fmt.Sprintf("detector/%s/%019d", sym, targetEpoch),
			}
			if fmt.Sprintf("%d", targetEpoch) != fmt.Sprintf("%019d", targetEpoch) {
				detectorPrefixes = append(detectorPrefixes, fmt.Sprintf("detector/%s/%d", sym, targetEpoch))
			}

			for measurement := range hindsight.Measurements(engine.store, detectorPrefixes...) {
				if system.Cfg.Audit.Ticks > 0 && currentTick > 0 {
					if measurement.Tick > currentTick {
						continue
					}
					metrics := data.Metrics(measurement)
					if endTick, ok := metrics["end_tick"]; ok {
						if uint64(endTick) > currentTick {
							continue
						}
					}
					if cTick, ok := metrics["c_tick"]; ok {
						if uint64(cTick) > currentTick {
							continue
						}
					}
				}

				for _, stage := range precursorStages {
					stage.Channel() <- measurement
				}

				for _, stage := range contractStages {
					stage.Channel() <- measurement
				}
			}

			// Stream policy decisions exclusively to CognitiveStage and ContractStage
			policyPrefixes := []string{
				fmt.Sprintf("policy/%s/%019d", sym, targetEpoch),
			}
			if fmt.Sprintf("%d", targetEpoch) != fmt.Sprintf("%019d", targetEpoch) {
				policyPrefixes = append(policyPrefixes, fmt.Sprintf("policy/%s/%d", sym, targetEpoch))
			}

			for measurement := range hindsight.Measurements(engine.store, policyPrefixes...) {
				for _, stage := range cognitiveStages {
					stage.Channel() <- measurement
				}

				for _, stage := range contractStages {
					stage.Channel() <- measurement
				}
			}
		}

		for _, stage := range engine.stages {
			close(stage.Channel())
		}

		return nil
	})

	if err := group.Wait(); err != nil {
		return errnie.Error(err)
	}

	var builder strings.Builder
	builder.WriteString("# Audit\n\n")

	for _, stage := range engine.stages {
		builder.WriteString("## ")
		builder.WriteString(stage.Name())
		builder.WriteString("\n")
		builder.WriteString(stage.Report())
		builder.WriteString("\n\n")
	}

	builder.WriteString("## Visualizations & Excursion Dynamics\n\n")
	builder.WriteString("![Excursion Dynamic Transitions Animation](plots/excursion_transitions.gif)\n\n")
	builder.WriteString("![Excursion Dynamic Transitions Summary](plots/excursion_transitions_summary.png)\n\n")
	builder.WriteString("![Audit Summary Scorecard](plots/audit_summary_scorecard.png)\n\n")

	outputDir := system.Cfg.Audit.OutputDir

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"failed to create audit output dir",
			err,
		))
	}

	reportPath := filepath.Join(outputDir, "audit.md")
	fh, err := os.Create(reportPath)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"failed to create audit report",
			err,
		))
	}

	defer func() {
		if err := fh.Close(); err != nil {
			errnie.Error(errnie.Err(errnie.IO, "failed to close audit report", err))
		}
	}()

	if _, err := fh.WriteString(builder.String()); err != nil {
		return errnie.Error(errnie.Err(errnie.IO, "failed to write audit report", err))
	}

	results := make(map[string]any)

	for _, stage := range engine.stages {
		stageKey := strings.TrimPrefix(stage.Name(), "audit:")
		results[stageKey] = stage.Data()
	}

	jsonBytes, err := json.MarshalIndent(results, "", "  ")

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"failed to marshal audit results json",
			err,
		))
	}

	jsonPath := filepath.Join(outputDir, "audit_results.json")
	err = os.WriteFile(jsonPath, jsonBytes, 0644)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"failed to write audit results json",
			err,
		))
	}

	if system.Cfg.Audit.NoPlots {
		return nil
	}

	plotCmd := exec.CommandContext(
		engine.Context(),
		"python3",
		"scripts/plot_audit.py",
		jsonPath,
		outputDir,
	)
	plotCmd.Stdout = os.Stdout
	plotCmd.Stderr = os.Stderr

	if err := plotCmd.Run(); err != nil {
		errnie.Error(errnie.Err(
			errnie.Internal,
			"audit plot rendering failed",
			err,
		))
	}

	return nil
}
