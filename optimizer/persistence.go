package optimizer

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bytedance/sonic"
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
	OutputPath string
	Seed       int64
	Iterations int
	OnBest     func(BestTree)
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

	search := tuner.newTreeSearch()

	if options.Iterations > 0 {
		search.iterations = options.Iterations
	}

	var writeErr error

	search.onBest = func(best BestTree) {
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

	tuner.branches = search.Run()

	if writeErr != nil {
		return SessionSummary{}, writeErr
	}

	return tuner.Summary(), nil
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
	Branches  []branchYAML               `yaml:"branches,omitempty"`
	Category  perspectives.CategoryType  `yaml:"category,omitempty"`
	Condition perspectives.ConditionType `yaml:"condition,omitempty"`
	Unit      perspectives.UnitType      `yaml:"unit,omitempty"`
	Value     *float64                   `yaml:"value,omitempty"`
	ValueSet  bool                       `yaml:"value_set,omitempty"`
	Action    *actionYAML                `yaml:"action,omitempty"`
}

type actionYAML struct {
	Type perspectives.ActionType `yaml:"type"`
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
		Category:  branch.Category,
		Condition: branch.Condition,
		Unit:      branch.Unit,
		ValueSet:  branch.ValueSet,
	}

	if branch.ValueSet {
		value := branch.Value
		document.Value = &value
	}

	if branch.Action.Type != perspectives.ActionNone {
		document.Action = &actionYAML{Type: branch.Action.Type}
	}

	if len(branch.Branches) > 0 {
		document.Branches = branchDocumentsFromBranches(
			perspectives.BranchList(branch.Branches),
		)
	}

	return document
}
