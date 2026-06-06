package analyze

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/bytedance/sonic"
)

// LiveMaxRows is the sliding tail window used for streaming diagnostics so each
// refresh stays bounded while still reflecting recent signal behaviour.
const LiveMaxRows = 50_000

/*
AnalyzeFileTail reads the most recent maxRows lines of a raw JSONL dump and returns
their diagnostic report. total_rows counts every non-empty line in the file; rows is
the number actually analyzed (the tail window). Live is set true on the report.
*/
func AnalyzeFileTail(signal, path string, maxRows int) (*Report, error) {
	if maxRows <= 0 {
		maxRows = LiveMaxRows
	}

	file, err := os.Open(path)

	if err != nil {
		return nil, fmt.Errorf("analyze: open %q: %w", path, err)
	}

	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)

	window := make([]string, 0, maxRows)
	totalRows := 0
	skipped := 0

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "" {
			continue
		}

		totalRows++

		window = append(window, line)

		if len(window) > maxRows {
			window = window[1:]
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("analyze: scan %q: %w", path, err)
	}

	accumulators := map[string]*accumulator{}
	fieldOrder := 0
	rows := 0

	for _, line := range window {
		object := map[string]any{}

		if err := sonic.Unmarshal([]byte(line), &object); err != nil {
			skipped++

			continue
		}

		rows++

		for key, value := range object {
			acc := accumulators[key]

			if acc == nil {
				acc = &accumulator{name: key, order: fieldOrder}
				fieldOrder++
				accumulators[key] = acc
			}

			acc.observe(value)
		}
	}

	ordered := make([]*accumulator, 0, len(accumulators))

	for _, acc := range accumulators {
		ordered = append(ordered, acc)
	}

	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].order < ordered[j].order
	})

	fields := make([]FieldReport, 0, len(ordered))

	for _, acc := range ordered {
		fields = append(fields, acc.report())
	}

	report := &Report{
		Signal:      signal,
		File:        path,
		Rows:        rows,
		TotalRows:   totalRows,
		Skipped:     skipped,
		Truncated:   totalRows > rows,
		Live:        true,
		Fields:      fields,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
	}

	report.Headline = headline(report)

	return report, nil
}
