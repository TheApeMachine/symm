package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	s3store "github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
	"golang.org/x/sync/errgroup"
)

type AuditorStage interface {
	Channel() chan *data.Measurement
	Run() error
	Report() string
	Name() string
	Data() any
}

type Violation struct {
	Symbol      string
	Name        string
	Description string
	Measurement *data.Measurement
	Metric      *data.Metric
}

func NewViolation(
	symbol, name, description string,
	measurement *data.Measurement,
	metric *data.Metric) *Violation {
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
	t.Execute(&b, violation)

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
		for page := range engine.store.List("run/epoch/") {
			for _, object := range page.Contents {
				chunks := strings.Split(*object.Key, "/")

				if len(chunks) > 2 {
					for _, source := range types.SignalSources {
						errnie.Info(fmt.Sprintf("%s: Streaming...", source))

						for measurement := range engine.store.Get(fmt.Sprintf(
							"%s/%s/%s", source, "BTC/USD", chunks[2],
						)) {
							for _, stage := range engine.stages {
								stage.Channel() <- measurement
							}
						}
					}
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

	defer fh.Close()
	fh.WriteString(builder.String())

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
