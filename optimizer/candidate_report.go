package optimizer

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

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
		ProfitLoss:  candidate.ProfitLoss(),
		ReturnPct:   candidate.ReturnPct(),
		BranchCount: candidate.BranchCount(),
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
