package replay

import (
	"bytes"
	"fmt"
	"os"
)

const (
	defaultHoldoutFraction = 0.20
	defaultSplitMinLines   = 200
)

/*
SplitHoldout writes the first (1-fraction) of source JSONL lines to trainPath and
the tail fraction to holdoutPath. Returns ok=false when the file is too short to
split without starving the train set.
*/
func SplitHoldout(
	sourcePath string,
	holdoutFraction float64,
	minLines int,
) (trainPath, holdoutPath string, ok bool, err error) {
	if holdoutFraction <= 0 || holdoutFraction >= 1 {
		return "", "", false, fmt.Errorf("holdout fraction must be between 0 and 1")
	}

	if minLines <= 0 {
		minLines = defaultSplitMinLines
	}

	payload, err := os.ReadFile(sourcePath)

	if err != nil {
		return "", "", false, err
	}

	lines := nonEmptyLines(payload)

	if len(lines) < minLines {
		return "", "", false, nil
	}

	holdoutCount := int(float64(len(lines)) * holdoutFraction)

	if holdoutCount < 1 {
		holdoutCount = 1
	}

	if holdoutCount >= len(lines) {
		return "", "", false, nil
	}

	splitAt := len(lines) - holdoutCount
	trainLines := lines[:splitAt]
	holdoutLines := lines[splitAt:]

	trainFile, err := os.CreateTemp("", "symm-train-*.jsonl")

	if err != nil {
		return "", "", false, err
	}

	holdoutFile, err := os.CreateTemp("", "symm-holdout-*.jsonl")

	if err != nil {
		_ = trainFile.Close()
		_ = os.Remove(trainFile.Name())

		return "", "", false, err
	}

	if err := writeLines(trainFile, trainLines); err != nil {
		_ = trainFile.Close()
		_ = holdoutFile.Close()
		_ = os.Remove(trainFile.Name())
		_ = os.Remove(holdoutFile.Name())

		return "", "", false, err
	}

	if err := writeLines(holdoutFile, holdoutLines); err != nil {
		_ = trainFile.Close()
		_ = holdoutFile.Close()
		_ = os.Remove(trainFile.Name())
		_ = os.Remove(holdoutFile.Name())

		return "", "", false, err
	}

	if err := trainFile.Close(); err != nil {
		_ = holdoutFile.Close()
		_ = os.Remove(trainFile.Name())
		_ = os.Remove(holdoutFile.Name())

		return "", "", false, err
	}

	if err := holdoutFile.Close(); err != nil {
		_ = os.Remove(trainFile.Name())
		_ = os.Remove(holdoutFile.Name())

		return "", "", false, err
	}

	return trainFile.Name(), holdoutFile.Name(), true, nil
}

/*
DefaultHoldoutFraction is the tail share reserved for holdout evals.
*/
func DefaultHoldoutFraction() float64 {
	return defaultHoldoutFraction
}

func nonEmptyLines(payload []byte) [][]byte {
	raw := bytes.Split(payload, []byte("\n"))
	lines := make([][]byte, 0, len(raw))

	for _, line := range raw {
		trimmed := bytes.TrimSpace(line)

		if len(trimmed) == 0 {
			continue
		}

		lines = append(lines, trimmed)
	}

	return lines
}

func writeLines(file *os.File, lines [][]byte) error {
	for index, line := range lines {
		if _, err := file.Write(line); err != nil {
			return err
		}

		if index < len(lines)-1 {
			if _, err := file.Write([]byte("\n")); err != nil {
				return err
			}
		}
	}

	if len(lines) > 0 {
		if _, err := file.Write([]byte("\n")); err != nil {
			return err
		}
	}

	return nil
}

/*
RemoveSplitFiles deletes temporary train/holdout files from SplitHoldout.
*/
func RemoveSplitFiles(paths ...string) {
	for _, path := range paths {
		if path == "" {
			continue
		}

		_ = os.Remove(path)
	}
}
