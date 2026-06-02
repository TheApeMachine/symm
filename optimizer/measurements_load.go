package optimizer

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/symm/market/perspectives"
)

func CountMeasurementLines(path string) (int, int, error) {
	return countValidMeasurementLines(path)
}

/*
LoadMeasurements reads the JSONL measurement tape written by market.Story.
When maxRows is positive and the file is larger, only an evenly spaced sample
is retained so multi-million-row captures do not load entirely into memory.
Malformed, truncated, or unparsable JSONL lines increment skipped.
*/
func LoadMeasurements(path string, maxRows int) ([]perspectives.Measurement, int, error) {
	if maxRows <= 0 {
		return loadAllMeasurements(path)
	}

	total, skipped, err := countValidMeasurementLines(path)

	if err != nil {
		return nil, skipped, err
	}

	if total <= maxRows {
		rows, loadSkipped, loadErr := loadAllMeasurements(path)

		return rows, skipped + loadSkipped, loadErr
	}

	TuneLog("subsampling %d measurements to %d rows", total, maxRows)

	rows, sampleSkipped, err := loadSampledMeasurements(path, total, maxRows)

	return rows, skipped + sampleSkipped, err
}

func loadAllMeasurements(path string) ([]perspectives.Measurement, int, error) {
	file, err := os.Open(path)

	if err != nil {
		return nil, 0, err
	}

	defer file.Close()

	rows := make([]perspectives.Measurement, 0)
	skipped := 0
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "" {
			continue
		}

		measurement := perspectives.Measurement{}

		if err := sonic.Unmarshal([]byte(line), &measurement); err != nil {
			skipped++

			continue
		}

		rows = append(rows, measurement)
	}

	if err := scanner.Err(); err != nil {
		return nil, skipped, err
	}

	if len(rows) == 0 {
		return nil, skipped, fmt.Errorf(
			"optimizer: no valid measurements in %s (skipped %d lines)",
			path, skipped,
		)
	}

	return rows, skipped, nil
}

func countValidMeasurementLines(path string) (int, int, error) {
	file, err := os.Open(path)

	if err != nil {
		return 0, 0, err
	}

	defer file.Close()

	total := 0
	skipped := 0
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "" {
			continue
		}

		measurement := perspectives.Measurement{}

		if err := sonic.Unmarshal([]byte(line), &measurement); err != nil {
			skipped++

			continue
		}

		total++
	}

	if err := scanner.Err(); err != nil {
		return 0, skipped, err
	}

	if total == 0 {
		return 0, skipped, fmt.Errorf(
			"optimizer: no valid measurements in %s (skipped %d lines)",
			path, skipped,
		)
	}

	return total, skipped, nil
}

func loadSampledMeasurements(
	path string, total, maxRows int,
) ([]perspectives.Measurement, int, error) {
	targets := make(map[int]struct{}, maxRows)
	lastIndex := total - 1
	step := float64(lastIndex) / float64(maxRows-1)

	for sampleIndex := range maxRows {
		index := int(math.Round(step * float64(sampleIndex)))

		if index > lastIndex {
			index = lastIndex
		}

		targets[index] = struct{}{}
	}

	file, err := os.Open(path)

	if err != nil {
		return nil, 0, err
	}

	defer file.Close()

	rows := make([]perspectives.Measurement, 0, maxRows)
	skipped := 0
	validIndex := 0
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "" {
			continue
		}

		measurement := perspectives.Measurement{}

		if err := sonic.Unmarshal([]byte(line), &measurement); err != nil {
			skipped++

			continue
		}

		if _, keep := targets[validIndex]; keep {
			rows = append(rows, measurement)
		}

		validIndex++
	}

	if err := scanner.Err(); err != nil {
		return nil, skipped, err
	}

	return rows, skipped, nil
}

/*
SubsampleMeasurements returns an evenly spaced subset capped at maxRows.
*/
func SubsampleMeasurements(
	rows []perspectives.Measurement, maxRows int,
) []perspectives.Measurement {
	if maxRows <= 0 || len(rows) <= maxRows {
		return rows
	}

	sampled := make([]perspectives.Measurement, 0, maxRows)
	lastIndex := len(rows) - 1
	step := float64(lastIndex) / float64(maxRows-1)

	for sampleIndex := range maxRows {
		index := int(math.Round(step * float64(sampleIndex)))

		if index > lastIndex {
			index = lastIndex
		}

		sampled = append(sampled, rows[index])
	}

	return sampled
}
