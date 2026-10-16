package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	s3store "github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/system"
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
  **METRIC:** {{.Metric}},
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
It reads one epoch of stored ticks (one full tick Value per key, under
data.TickPrefix), the excursions detect wrote and the policies train
published for it, fans every symbol's measurements out to the stages, and
writes the report and artifacts.
*/
type Engine struct {
	*runtime.System
	store  *s3store.Client
	stages []AuditorStage
}

/*
NewEngine constructs an audit engine reading through client, configured by
system.Cfg.Audit.
*/
func NewEngine(ctx context.Context, client *s3store.Client) (*Engine, error) {
	if client == nil {
		return nil, errnie.Err(errnie.Validation, "[audit:engine] a store client is required", nil)
	}

	if err := validateConfig(system.Cfg.Audit); err != nil {
		return nil, err
	}

	return &Engine{
		System: runtime.NewSystem(ctx, "audit:engine"),
		store:  client,
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
	}, nil
}

/*
validateConfig rejects an audit configuration a stage cannot judge by: no
stage falls back to a value of its own.
*/
func validateConfig(cfg *system.Audit) error {
	checks := []struct {
		name     string
		value    float64
		min, max float64
		openMin  bool
	}{
		{"audit.permutations", float64(cfg.Permutations), 1, math.MaxInt32, false},
		{"audit.significance", cfg.Significance, 0, 1, true},
		{"audit.taker_fee (--taker-fee-percent / 100)", cfg.TakerFee, 0, 1, true},
		{"audit.min_tokens", float64(cfg.MinTokens), 2, math.MaxInt32, false},
		{"audit.min_precursor_tokens", float64(cfg.MinPrecursorTokens), 1, math.MaxInt32, false},
		{"audit.dominance_share", cfg.DominanceShare, 0, 1, true},
		{"audit.entropy_tolerance", cfg.EntropyTolerance, 0, math.MaxFloat64, true},
		{"audit.stationarity_jsd", cfg.StationarityJSD, 0, 1, true},
		{"audit.grid_completeness", cfg.GridCompleteness, 0, 1, true},
		{"audit.stagnant_zero_fraction", cfg.StagnantZeroFraction, 0, 1, true},
		{"audit.identity_tolerance", cfg.IdentityTolerance, 0, math.MaxFloat64, true},
		{"audit.latency_dispersion_floor_ms", cfg.LatencyDispersionFloorMs, 0, math.MaxFloat64, true},
		{"audit.latency_spike_floor_ms", cfg.LatencySpikeFloorMs, 0, math.MaxFloat64, true},
		{"audit.collinear_correlation", cfg.CollinearCorrelation, 0, 1, true},
		{"audit.comovement_concordance", cfg.ComovementConcordance, 0, 1, true},
	}

	for _, check := range checks {
		low := check.value < check.min || (check.openMin && check.value == check.min)

		if math.IsNaN(check.value) || low || check.value > check.max {
			return errnie.Err(errnie.Validation, fmt.Sprintf(
				"[audit:engine] %s = %v is outside its domain (%v, %v]", check.name, check.value, check.min, check.max,
			), nil)
		}
	}

	if cfg.Significance >= 1 || cfg.TakerFee >= 1 {
		return errnie.Err(errnie.Validation, "[audit:engine] audit.significance and the taker fee must be below 1", nil)
	}

	return nil
}

/*
resolveTargetEpoch is system.Cfg.Audit.Epoch when set, else the newest epoch
with stored ticks.
*/
func (engine *Engine) resolveTargetEpoch() (uint64, error) {
	if system.Cfg.Audit.Epoch > 0 {
		return uint64(system.Cfg.Audit.Epoch), nil
	}

	epochs, err := s3store.Epochs(engine.store, data.TickPrefix())

	if err != nil {
		return 0, err
	}

	if len(epochs) == 0 {
		return 0, errnie.Err(errnie.NotFound, "[audit:engine] no stored ticks under "+data.TickPrefix(), nil)
	}

	return epochs[len(epochs)-1], nil
}

/*
Routes name the stages each kind of measurement is fanned out to.
*/
type routes struct {
	trades, signals, prices, joins, excursions, policies []AuditorStage
}

func (engine *Engine) routes() routes {
	stage := make(map[string]AuditorStage, len(engine.stages))

	for _, s := range engine.stages {
		stage[s.Name()] = s
	}

	pick := func(names ...string) []AuditorStage {
		out := make([]AuditorStage, 0, len(names))

		for _, name := range names {
			out = append(out, stage[name])
		}

		return out
	}

	return routes{
		trades:     pick("audit:timing", "audit:contract", "audit:truthfulness"),
		signals:    pick("audit:contract", "audit:vitality", "audit:redundancy", "audit:sympathy", "audit:truthfulness"),
		prices:     pick("audit:contract", "audit:vitality", "audit:sympathy", "audit:truthfulness"),
		joins:      pick("audit:stability", "audit:dynamics", "audit:precursor"),
		excursions: pick("audit:precursor", "audit:contract"),
		policies:   pick("audit:cognitive", "audit:contract"),
	}
}

func send(stages []AuditorStage, measurement *data.Value) {
	for _, stage := range stages {
		stage.Channel() <- measurement
	}
}

/*
feed streams epoch to the stages and closes their channels when done.
*/
func (engine *Engine) feed(epoch uint64) error {
	defer func() {
		for _, stage := range engine.stages {
			close(stage.Channel())
		}
	}()

	route := engine.routes()
	symbols := make([]string, 0)
	var ticks int
	var lastTick uint64

	for value := range s3store.Values(engine.store, data.TickPrefix(epoch)) {
		if err := engine.Context().Err(); err != nil {
			return err
		}

		views, err := TickViews(value, system.Cfg.Audit.Symbol)

		if err != nil {
			return err
		}

		ticks++
		lastTick = value.Tick

		for _, view := range views {
			if !slices.Contains(symbols, view.Symbol) {
				symbols = append(symbols, view.Symbol)
			}

			for _, trade := range view.Trades {
				send(route.trades, trade)
			}

			for _, signal := range view.Signals {
				send(route.signals, signal)
			}

			send(route.prices, view.Price)
			send(route.joins, view.Join)
		}

		if system.Cfg.Audit.Ticks > 0 && ticks >= system.Cfg.Audit.Ticks {
			break
		}
	}

	if ticks == 0 {
		return errnie.Err(errnie.NotFound, "[audit:engine] no stored ticks under "+data.TickPrefix(epoch), nil)
	}

	errnie.Info(fmt.Sprintf("[audit:engine] epoch %d: %d ticks (through tick %d), symbols %v", epoch, ticks, lastTick, symbols))

	var excursions, policies int

	for _, symbol := range symbols {
		for excursion := range s3store.Values(engine.store, strategy.ExcursionPrefixes(symbol, epoch)...) {
			// An excursion that ends after the audited ticks is outside the
			// window the stages saw.
			if excursion.Tick > lastTick {
				continue
			}

			excursions++
			send(route.excursions, excursion)
		}
	}

	for policy := range s3store.Values(engine.store, strategy.PolicyPrefixes(epoch)...) {
		policies++
		send(route.policies, policy)
	}

	errnie.Info(fmt.Sprintf("[audit:engine] epoch %d: %d excursions, %d policies", epoch, excursions, policies))

	return nil
}

/*
Run executes every audit stage over one epoch and writes audit.md and
audit_results.json (and plots unless disabled).
*/
func (engine *Engine) Run() error {
	epoch, err := engine.resolveTargetEpoch()

	if err != nil {
		return errnie.Error(err)
	}

	group, _ := errgroup.WithContext(engine.Context())

	for _, stage := range engine.stages {
		group.Go(func() error {
			return stage.Run()
		})
	}

	group.Go(func() error {
		return engine.feed(epoch)
	})

	if err := group.Wait(); err != nil {
		return errnie.Error(err)
	}

	results := make(map[string]any)
	failed := make([]string, 0)

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("# Audit of epoch %d (%s)\n\n", epoch, data.TickPrefix(epoch)))
	builder.WriteString("| stage | result |\n|---|---|\n")

	for _, stage := range engine.stages {
		stageKey := strings.TrimPrefix(stage.Name(), "audit:")
		stageData := stage.Data()
		results[stageKey] = stageData
		result := "FAIL"

		if fields, ok := stageData.(map[string]any); ok && fields["passed"] == true {
			result = "pass"
		} else {
			failed = append(failed, stageKey)
		}

		builder.WriteString(fmt.Sprintf("| %s | %s |\n", stageKey, result))
	}

	builder.WriteString("\n")
	results["epoch"] = epoch
	results["failed_stages"] = failed
	errnie.Info(fmt.Sprintf("[audit:engine] epoch %d: %d of %d stages failed %v", epoch, len(failed), len(engine.stages), failed))

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
