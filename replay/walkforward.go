package replay

import (
	"fmt"
	"os"
)

const (
	defaultWalkForwardFolds = 3
)

/*
WalkForwardHoldouts writes expanding-window holdout tails for walk-forward tuning.
Each fold reserves an additional holdoutFraction/folds slice of the capture timeline.
*/
func WalkForwardHoldouts(
	sourcePath string,
	folds int,
	holdoutFraction float64,
) (holdoutPaths []string, cleanup func(), err error) {
	if folds <= 1 {
		return nil, func() {}, nil
	}

	if holdoutFraction <= 0 || holdoutFraction >= 1 {
		return nil, func() {}, fmt.Errorf("holdout fraction must be between 0 and 1")
	}

	payload, err := os.ReadFile(sourcePath)

	if err != nil {
		return nil, func() {}, err
	}

	lines := nonEmptyLines(payload)

	if len(lines) < defaultSplitMinLines {
		return nil, func() {}, nil
	}

	totalHoldout := int(float64(len(lines)) * holdoutFraction)

	if totalHoldout < folds {
		return nil, func() {}, nil
	}

	foldSize := totalHoldout / folds

	if foldSize < 1 {
		return nil, func() {}, nil
	}

	paths := make([]string, 0, folds)
	created := make([]string, 0, folds)

	for fold := 1; fold <= folds; fold++ {
		holdoutStart := len(lines) - foldSize*fold

		if holdoutStart <= 0 || holdoutStart >= len(lines) {
			break
		}

		holdoutLines := lines[holdoutStart:]
		holdoutFile, fileErr := os.CreateTemp("", fmt.Sprintf("symm-wf-holdout-%d-*.jsonl", fold))

		if fileErr != nil {
			RemoveSplitFiles(created...)

			return nil, func() {}, fileErr
		}

		if writeErr := writeLines(holdoutFile, holdoutLines); writeErr != nil {
			_ = holdoutFile.Close()
			RemoveSplitFiles(append(created, holdoutFile.Name())...)

			return nil, func() {}, writeErr
		}

		if closeErr := holdoutFile.Close(); closeErr != nil {
			RemoveSplitFiles(append(created, holdoutFile.Name())...)

			return nil, func() {}, closeErr
		}

		created = append(created, holdoutFile.Name())
		paths = append(paths, holdoutFile.Name())
	}

	if len(paths) == 0 {
		return nil, func() {}, nil
	}

	return paths, func() { RemoveSplitFiles(created...) }, nil
}

/*
DefaultWalkForwardFolds is the walk-forward holdout count used when tuning.
*/
func DefaultWalkForwardFolds() int {
	return defaultWalkForwardFolds
}
