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
	AuditEvent  string `json:"audit_event"`
	Symbol      string `json:"symbol"`
	Verdict     any    `json:"verdict"`
	BlockReason string `json:"block_reason"`
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
