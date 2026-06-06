package io

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func CountMeasurementLines(path string) (int, int, error) {
	return countValidMeasurementLines(path)
}

/*
LoadMeasurements reads the JSONL measurement tape written by market.Story.
Malformed, truncated, or unparsable JSONL lines increment skipped.
*/
func LoadMeasurements(path string) ([]types.Measurement, int, error) {
	return loadAllMeasurements(path)
}

func loadAllMeasurements(path string) ([]types.Measurement, int, error) {
	file, err := os.Open(path)

	if err != nil {
		return nil, 0, err
	}

	defer file.Close()

	rows := make([]types.Measurement, 0)
	skipped := 0
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "" {
			continue
		}

		measurement := types.Measurement{}

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

		measurement := types.Measurement{}

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
