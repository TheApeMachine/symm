package integration

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
)

/*
AuditRow is one JSONL frame from trading.audit.file.
*/
type AuditRow struct {
	AuditEvent           string  `json:"audit_event"`
	Symbol               string  `json:"symbol"`
	Type                 string  `json:"type"`
	Side                 string  `json:"side"`
	Verdict              string  `json:"verdict"`
	BlockReason          string  `json:"block_reason"`
	PreflightGate        string  `json:"preflight_gate"`
	Price                float64 `json:"price"`
	Quantity             float64 `json:"quantity"`
	Offset               float64 `json:"offset"`
	Fraction             float64 `json:"fraction"`
	EntryPrice           float64 `json:"entry_price"`
	ExitPrice            float64 `json:"exit_price"`
	Fee                  float64 `json:"fee"`
	RealizedPnL          float64 `json:"realized_pnl"`
	SpreadBps            float64 `json:"spread_bps"`
	QuoteAgeMs           int64   `json:"quote_age_ms"`
	DepthCoverage        float64 `json:"depth_coverage"`
	ProjectedSlippageBps float64 `json:"projected_slippage_bps"`
	Regime               string  `json:"regime"`
	Source               string  `json:"source"`
	Category             string  `json:"category"`
	SNR                  float64 `json:"snr"`
}

func readAuditRows(path string) ([]AuditRow, error) {
	if path == "" {
		return nil, nil
	}

	file, err := os.Open(path)

	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}

		return nil, fmt.Errorf("integration audit: open %q: %w", path, err)
	}

	defer file.Close()

	rows := make([]AuditRow, 0, 32)
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		row := AuditRow{}

		if err := json.Unmarshal(scanner.Bytes(), &row); err != nil {
			return nil, fmt.Errorf("integration audit: parse: %w", err)
		}

		rows = append(rows, row)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("integration audit: scan: %w", err)
	}

	return rows, nil
}
