package optimizer

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/market/perspectives"
	"go.yaml.in/yaml/v3"
)

/*
BestTree is one improved tree found during a search.
*/
type BestTree struct {
	Iteration int
	Score     float64
	Branches  perspectives.BranchList
}

/*
TuneOptions controls a measurement-backed optimizer run.
*/
type TuneOptions struct {
	OutputPath          string
	CandidateReportPath string
	Seed                int64
	Workers             int
	MaxThresholds       int
	BeamWidth           int
	CandidateLimit      int
	OnBest              func(BestTree)
}

/*
CandidateScore is one scored candidate tree emitted by the scanner.
*/
type CandidateScore struct {
	Candidate int
	Score     float64
	Branches  perspectives.BranchList
}

/*
LoadMeasurements reads the JSONL measurement tape written by market.Story.
*/
func LoadMeasurements(path string) ([]perspectives.Measurement, error) {
	file, err := os.Open(path)

	if err != nil {
		return nil, err
	}

	defer file.Close()

	rows := make([]perspectives.Measurement, 0)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		line := strings.TrimSpace(scanner.Text())

		if line == "" {
			continue
		}

		measurement := perspectives.Measurement{}

		if err := sonic.Unmarshal([]byte(line), &measurement); err != nil {
			return nil, fmt.Errorf("optimizer: measurement line %d: %w", lineNumber, err)
		}

		rows = append(rows, measurement)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return rows, nil
}

/*
TuneMeasurements searches trees against a recorded measurement tape.
*/
func TuneMeasurements(
	ctx context.Context,
	rows []perspectives.Measurement,
	options TuneOptions,
) (SessionSummary, error) {
	seed := options.Seed

	if seed == 0 {
		seed = time.Now().UnixNano()
	}

	tuner := &Tuner{
		ctx:     ctx,
		profile: Profile{ctx: ctx},
		seed:    seed,
	}

	for _, row := range rows {
		tuner.profile.Add(row)
	}

	reporter, err := newCandidateReporter(options.CandidateReportPath)

	if err != nil {
		return SessionSummary{}, err
	}

	var writeErr error
	onBest := func(best BestTree) {
		if options.OutputPath != "" {
			if err := WriteBranches(options.OutputPath, best.Branches); err != nil {
				writeErr = err

				return
			}
		}

		if options.OnBest != nil {
			options.OnBest(best)
		}
	}
	onCandidate := func(candidate CandidateScore) {
		if reporter == nil || writeErr != nil {
			return
		}

		if err := reporter.Write(candidate); err != nil {
			writeErr = err
		}
	}

	search := NewScanSearch(
		ctx,
		&tuner.profile,
		rows,
		ScanOptions{
			Workers:        options.Workers,
			MaxThresholds:  options.MaxThresholds,
			BeamWidth:      options.BeamWidth,
			CandidateLimit: options.CandidateLimit,
		},
	)
	search.onBest = onBest
	search.onCandidate = onCandidate

	var stats ScanStats
	tuner.branches, stats = search.Run()

	if reporter != nil {
		if err := reporter.Close(); err != nil && writeErr == nil {
			writeErr = err
		}
	}

	if writeErr != nil {
		return SessionSummary{}, writeErr
	}

	summary := tuner.Summary()
	summary.Candidates = stats.Candidates
	summary.Workers = stats.Workers

	return summary, nil
}

/*
WriteBranches writes the optimizer tree document atomically.
*/
func WriteBranches(path string, branches perspectives.BranchList) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("optimizer: empty perspectives output path")
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	document := branchDocument{
		Version: 1,
		Branches: branchDocumentsFromBranches(
			branches,
		),
	}

	raw, err := yaml.Marshal(document)

	if err != nil {
		return err
	}

	temp, err := os.CreateTemp(filepath.Dir(path), ".perspectives-*.yaml")

	if err != nil {
		return err
	}

	tempPath := temp.Name()

	if _, err := temp.Write(raw); err != nil {
		temp.Close()
		os.Remove(tempPath)

		return err
	}

	if err := temp.Close(); err != nil {
		os.Remove(tempPath)

		return err
	}

	return os.Rename(tempPath, path)
}

type branchDocument struct {
	Version  int          `yaml:"version"`
	Branches []branchYAML `yaml:"branches"`
}

type branchYAML struct {
	Branches    []branchYAML                 `yaml:"branches,omitempty" json:"branches,omitempty"`
	Category    perspectives.CategoryType    `yaml:"category,omitempty" json:"category,omitempty"`
	Observation perspectives.ObservationType `yaml:"observation,omitempty" json:"observation,omitempty"`
	Metric      string                       `yaml:"metric,omitempty" json:"metric,omitempty"`
	Regime      perspectives.Regime          `yaml:"regime,omitempty" json:"regime,omitempty"`
	Condition   perspectives.ConditionType   `yaml:"condition,omitempty" json:"condition,omitempty"`
	Unit        perspectives.UnitType        `yaml:"unit,omitempty" json:"unit,omitempty"`
	Value       *float64                     `yaml:"value,omitempty" json:"value,omitempty"`
	ValueSet    bool                         `yaml:"value_set,omitempty" json:"value_set,omitempty"`
	Action      *actionYAML                  `yaml:"action,omitempty" json:"action,omitempty"`
}

type actionYAML struct {
	Type     perspectives.ActionType `yaml:"type" json:"type"`
	Side     trading.Side            `yaml:"side,omitempty" json:"side,omitempty"`
	Symbol   string                  `yaml:"symbol,omitempty" json:"symbol,omitempty"`
	Price    float64                 `yaml:"price,omitempty" json:"price,omitempty"`
	Quantity float64                 `yaml:"quantity,omitempty" json:"quantity,omitempty"`
}

func branchDocumentsFromBranches(
	branches perspectives.BranchList,
) []branchYAML {
	documents := make([]branchYAML, len(branches))

	for index, branch := range branches {
		documents[index] = branchDocumentFromBranch(branch)
	}

	return documents
}

func branchDocumentFromBranch(branch perspectives.Branch) branchYAML {
	document := branchYAML{
		Category:    branch.Category,
		Observation: branch.Observation,
		Metric:      branch.Metric,
		Regime:      branch.Regime,
		Condition:   branch.Condition,
		Unit:        branch.Unit,
		ValueSet:    branch.ValueSet,
	}

	if branch.ValueSet {
		value := branch.Value
		document.Value = &value
	}

	if hasAction(branch.Action) {
		document.Action = actionDocumentFromAction(branch.Action)
	}

	if len(branch.Branches) > 0 {
		document.Branches = branchDocumentsFromBranches(
			perspectives.BranchList(branch.Branches),
		)
	}

	return document
}

func actionDocumentFromAction(action perspectives.Action) *actionYAML {
	return &actionYAML{
		Type:     action.Type,
		Side:     action.Side,
		Symbol:   action.Symbol,
		Price:    action.Price,
		Quantity: action.Quantity,
	}
}

func hasAction(action perspectives.Action) bool {
	return action.Type != perspectives.ActionNone ||
		action.Side != "" ||
		action.Symbol != "" ||
		action.Price != 0 ||
		action.Quantity != 0
}

type candidateReport struct {
	Candidate   int          `json:"candidate"`
	Score       float64      `json:"score"`
	ProfitLoss  float64      `json:"profit_loss"`
	ReturnPct   float64      `json:"return_pct"`
	BranchCount int          `json:"branch_count"`
	Branches    []branchYAML `json:"branches"`
}

type candidateReporter struct {
	file    *os.File
	writer  *bufio.Writer
	encoder *json.Encoder
}

func newCandidateReporter(path string) (*candidateReporter, error) {
	if strings.TrimSpace(path) == "" {
		return nil, nil
	}

	if strings.TrimSpace(path) == "-" {
		writer := bufio.NewWriter(os.Stdout)
		encoder := json.NewEncoder(writer)
		encoder.SetEscapeHTML(false)

		return &candidateReporter{
			writer:  writer,
			encoder: encoder,
		}, nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}

	file, err := os.Create(path)

	if err != nil {
		return nil, err
	}

	writer := bufio.NewWriter(file)
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)

	return &candidateReporter{
		file:    file,
		writer:  writer,
		encoder: encoder,
	}, nil
}

func (reporter *candidateReporter) Write(candidate CandidateScore) error {
	return reporter.encoder.Encode(candidateReport{
		Candidate:   candidate.Candidate,
		Score:       candidate.Score,
		ProfitLoss:  candidate.Score,
		ReturnPct:   candidate.Score * 100,
		BranchCount: countBranches(candidate.Branches),
		Branches: branchDocumentsFromBranches(
			candidate.Branches,
		),
	})
}

func (reporter *candidateReporter) Close() error {
	flushErr := reporter.writer.Flush()

	if reporter.file == nil {
		return flushErr
	}

	closeErr := reporter.file.Close()

	if flushErr != nil {
		return flushErr
	}

	return closeErr
}

func countBranches(branches perspectives.BranchList) int {
	count := 0

	for _, branch := range branches {
		count++
		count += countBranches(perspectives.BranchList(branch.Branches))
	}

	return count
}
