package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"iter"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	s3store "github.com/theapemachine/symm/store"
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
	store           *s3store.Client
	stages          []AuditorStage
	currentPolicies int
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
			NewRealizationStage(ctx),
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
		{"audit.active_symbols", float64(cfg.ActiveSymbols), 1, math.MaxInt32, false},
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

	epochs, err := epochsUnder(engine.store, "workspace/tick/aggregate")

	if err != nil {
		return 0, err
	}

	if len(epochs) == 0 {
		return 0, errnie.Err(errnie.NotFound, "[audit:engine] no stored ticks under "+"workspace/tick/aggregate", nil)
	}

	return epochs[len(epochs)-1], nil
}

/*
resolvePolicy is the policy epoch the stages judge: system.Cfg.Audit.PolicyEpoch
when set (it must be trained on epoch's ticks), else the newest policy epoch
trained on them; none is the zero gate (the policy stages report it).
*/
func (engine *Engine) resolvePolicy(epoch uint64) (policyGate, error) {
	gates, err := policyGates(engine.store)

	if err != nil {
		return policyGate{}, err
	}

	var chosen policyGate

	for _, gate := range gates {
		if want := uint64(system.Cfg.Audit.PolicyEpoch); want > 0 {
			if gate.Epoch != want {
				continue
			}

			if gate.TickEpoch != epoch {
				return policyGate{}, errnie.Err(errnie.Validation, fmt.Sprintf(
					"[audit:engine] policy epoch %d was trained on tick epoch %d, not the audited %d", want, gate.TickEpoch, epoch,
				), nil)
			}

			chosen = gate
			break
		}

		if gate.TickEpoch == epoch && gate.Epoch > chosen.Epoch {
			chosen = gate
		}
	}

	if want := system.Cfg.Audit.PolicyEpoch; want > 0 && chosen.Epoch == 0 {
		return policyGate{}, errnie.Err(errnie.NotFound, fmt.Sprintf("[audit:engine] no policy epoch %d is published", want), nil)
	}

	if chosen.Epoch == 0 {
		errnie.Warn(fmt.Sprintf("[audit:engine] no policy epoch is trained on tick epoch %d", epoch))
	} else {
		errnie.Info("[audit:engine] judging policies of " + chosen.String())
	}

	return chosen, nil
}

/*
Routes name the stages each kind of measurement is fanned out to.
*/
type routes struct {
	trades, signals, prices, joins, excursions, policies, sequences, papers []AuditorStage
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
		policies:   pick("audit:cognitive", "audit:contract", "audit:realization"),
		sequences:  pick("audit:cognitive"),
		papers:     pick("audit:realization"),
	}
}

func send(stages []AuditorStage, measurement *data.Value) {
	for _, stage := range stages {
		stage.Channel() <- measurement
	}
}

/*
epochsUnder lists the epoch segments stored under prefix, oldest first.
*/
func epochsUnder(client *s3store.Client, prefix string) ([]uint64, error) {
	if client == nil {
		return nil, errnie.Err(errnie.Validation, "[audit] no store", nil)
	}

	seen := map[uint64]struct{}{}
	epochs := make([]uint64, 0)

	for key := range client.ListKeys(prefix) {
		for _, segment := range strings.Split(key, "/") {
			if len(segment) != 19 {
				continue
			}

			epoch, err := strconv.ParseUint(segment, 10, 64)

			if err != nil {
				continue
			}

			if _, ok := seen[epoch]; ok {
				continue
			}

			seen[epoch] = struct{}{}
			epochs = append(epochs, epoch)
		}
	}

	slices.Sort(epochs)

	return epochs, nil
}

/*
policyGates reads the gate on the first policy of every published policy epoch.
*/
func policyGates(client *s3store.Client) ([]policyGate, error) {
	all := make([]uint64, 0)

	for _, prefix := range policyPrefixes() {
		found, err := epochsUnder(client, prefix)

		if err != nil {
			return nil, errnie.Err(errnie.IO, "[audit] listing policy epochs under "+prefix, err)
		}

		all = append(all, found...)
	}

	slices.Sort(all)
	gates := make([]policyGate, 0)

	for _, epoch := range slices.Compact(all) {
		var gate *policyGate

		for value, err := range copyValues(client, policyPrefixes(epoch)...) {
			if err != nil {
				return nil, err
			}

			read, err := readPolicyGate(value)

			if err != nil {
				return nil, err
			}

			gate = &read

			break
		}

		if gate == nil {
			return nil, errnie.Err(errnie.NotFound, fmt.Sprintf("[audit] policy epoch %d lists no readable policy", epoch), nil)
		}

		gates = append(gates, *gate)
	}

	return gates, nil
}

/*
currentSequences reads the tree train saved at policy/current. The grammar
path lives in the object key, so a value that does not carry it is tagged
from that key.
*/
func currentSequences(client *s3store.Client) iter.Seq2[*data.Value, error] {
	return func(yield func(*data.Value, error) bool) {
		if client == nil {
			return
		}

		keys := make([]string, 0)

		for key := range client.ListKeys("policy/current/") {
			if strings.HasPrefix(key, "policy/current/") && strings.HasSuffix(key, ".bin") {
				keys = append(keys, key)
			}
		}

		slices.Sort(keys)

		for _, key := range keys {
			var buf bytes.Buffer
			err := client.Stream(context.Background(), key, &buf)

			if err != nil {
				if !yield(nil, err) {
					return
				}

				continue
			}

			if buf.Len() == 0 {
				continue
			}

			value := data.From(buf.Bytes())
			stampCurrentPath(value, key)

			if !yield(value, nil) {
				return
			}
		}
	}
}

func stampCurrentPath(value *data.Value, key string) {
	if _, err := policyPath(value); err == nil {
		return
	}

	path := pathFromCurrentKey(key)

	if path == "" || len(value.Index) == 0 {
		return
	}

	value.Index[len(value.Index)-1] += "/" + "path:" + path
}

func pathFromCurrentKey(key string) string {
	rest := strings.TrimSuffix(strings.TrimPrefix(key, "policy/current/"), ".bin")
	parts := strings.Split(rest, "/")

	if len(parts) < 2 {
		return ""
	}

	action := parts[len(parts)-1]

	if action != "enter" && action != "exit" && action != "noop" {
		return ""
	}

	return strings.Join(parts[:len(parts)-1], "/")
}

/*
copyValues reconstructs every object under prefixes, in key order.
*/
func copyValues(client *s3store.Client, prefixes ...string) iter.Seq2[*data.Value, error] {
	return func(yield func(*data.Value, error) bool) {
		if client == nil {
			return
		}

		for _, prefix := range prefixes {
			for key := range client.ListKeys(prefix) {
				var buf bytes.Buffer

				if err := client.Stream(context.Background(), key, &buf); err != nil {
					if !yield(nil, err) {
						return
					}

					continue
				}

				if buf.Len() == 0 {
					continue
				}

				if !yield(data.From(buf.Bytes()), nil) {
					return
				}
			}
		}
	}
}

/*
feed streams epoch to the stages and closes their channels when done.
*/
func (engine *Engine) feed(epoch uint64, policy policyGate) error {
	defer func() {
		for _, stage := range engine.stages {
			close(stage.Channel())
		}
	}()

	for _, stage := range engine.stages {
		if cognitive, ok := stage.(*CognitiveStage); ok {
			cognitive.policyEpoch = policy.Epoch
		}
	}

	route := engine.routes()
	symbols := make([]string, 0)
	var ticks int
	var lastTick uint64

	for value, err := range copyValues(engine.store, fmt.Sprintf("workspace/tick/aggregate/%019d", epoch)) {
		if err != nil {
			return err
		}

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
		return errnie.Err(errnie.NotFound, "[audit:engine] no stored ticks under "+fmt.Sprintf("workspace/tick/aggregate/%019d", epoch), nil)
	}

	errnie.Info(fmt.Sprintf("[audit:engine] epoch %d: %d ticks (through tick %d), symbols %v", epoch, ticks, lastTick, symbols))

	var excursions, policies int

	for _, symbol := range symbols {
		for excursion, err := range copyValues(engine.store, excursionPrefixes(symbol, epoch)...) {
			if err != nil {
				return err
			}

			// An excursion that ends after the audited ticks is outside the
			// window the stages saw.
			if excursion.Tick > lastTick {
				continue
			}

			excursions++
			send(route.excursions, excursion)
		}
	}

	if policy.Epoch != 0 {
		for value, err := range copyValues(engine.store, policyPrefixes(policy.Epoch)...) {
			if err != nil {
				return err
			}

			policies++
			send(route.policies, value)
		}
	} else {
		// train stores the tree at policy/current/<path>/<action>.bin. That
		// shelf has no tick-epoch gate, so only the cognitive stage reads it.
		for value, err := range currentSequences(engine.store) {
			if err != nil {
				return err
			}

			if value == nil || value.Origin != "policy" {
				continue
			}

			policies++
			engine.currentPolicies++
			send(route.sequences, value)
		}
	}

	// Paper records of every run; the realization stage judges those
	// traded under this epoch's policies.
	var papers int

	for paper, err := range copyValues(engine.store, paperActionPrefixes()...) {
		if err != nil {
			return err
		}

		papers++
		send(route.papers, paper)
	}

	errnie.Info(fmt.Sprintf("[audit:engine] epoch %d: %d excursions, %d policies, %d paper records", epoch, excursions, policies, papers))

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

	policy, err := engine.resolvePolicy(epoch)

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
		return engine.feed(epoch, policy)
	})

	if err := group.Wait(); err != nil {
		return errnie.Error(err)
	}

	results := make(map[string]any)
	failed := make([]string, 0)

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("# Audit of epoch %d (%s)\n\n", epoch, fmt.Sprintf("workspace/tick/aggregate/%019d", epoch)))

	switch {
	case policy.Epoch != 0:
		builder.WriteString(fmt.Sprintf("Policies: %s.\n\n", policy))
	case engine.currentPolicies > 0:
		builder.WriteString(fmt.Sprintf("%d sequences from policy/current.\n\n", engine.currentPolicies))
	default:
		builder.WriteString("No policy epoch is trained on these ticks.\n\n")
	}

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
