package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

type auditRow map[string]any

func main() {
	path := "runs/audit.jsonl"

	if len(os.Args) > 1 {
		path = os.Args[1]
	}

	rows, err := readRows(path)

	if err != nil {
		fmt.Fprintf(os.Stderr, "auditreport: %v\n", err)
		os.Exit(1)
	}

	if len(rows) == 0 {
		fmt.Println("no audit rows in", path)
		return
	}

	byEvent := countField(rows, "audit_event")
	byVerdict := countField(rows, "verdict")
	byGate := countField(rows, "preflight_gate")

	fmt.Printf("audit rows: %d\n\n", len(rows))

	printCounts("events", byEvent)
	printCounts("verdicts", byVerdict)
	printCounts("preflight gates (rejected)", byGate)

	var realizedSum float64
	var outcomes int

	for _, row := range rows {
		if row["audit_event"] != "position_outcome" {
			continue
		}

		outcomes++

		if value, ok := row["realized_pnl"].(float64); ok {
			realizedSum += value
		}
	}

	if outcomes > 0 {
		fmt.Printf("\nclosed positions: %d\n", outcomes)
		fmt.Printf("sum realized_pnl: %.4f EUR\n", realizedSum)
	}

	printSpreadRejections(rows)
}

func readRows(path string) ([]auditRow, error) {
	file, err := os.Open(path)

	if err != nil {
		return nil, err
	}
	defer file.Close()

	rows := make([]auditRow, 0, 1024)
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		row := auditRow{}

		if err := json.Unmarshal(scanner.Bytes(), &row); err != nil {
			return nil, err
		}

		rows = append(rows, row)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return rows, nil
}

func countField(rows []auditRow, key string) map[string]int {
	counts := make(map[string]int)

	for _, row := range rows {
		value, ok := row[key].(string)

		if !ok || value == "" {
			continue
		}

		counts[value]++
	}

	return counts
}

func printCounts(title string, counts map[string]int) {
	if len(counts) == 0 {
		return
	}

	keys := make([]string, 0, len(counts))

	for key := range counts {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	fmt.Printf("%s:\n", title)

	for _, key := range keys {
		fmt.Printf("  %s: %d\n", key, counts[key])
	}

	fmt.Println()
}

func printSpreadRejections(rows []auditRow) {
	spreads := make([]float64, 0)

	for _, row := range rows {
		if row["audit_event"] != "trade_decision" {
			continue
		}

		if row["verdict"] != "rejected" {
			continue
		}

		if row["preflight_gate"] != "spread" {
			continue
		}

		if spread, ok := row["spread_bps"].(float64); ok {
			spreads = append(spreads, spread)
		}
	}

	if len(spreads) == 0 {
		return
	}

	sort.Float64s(spreads)

	fmt.Printf("spread rejections with spread_bps captured: %d\n", len(spreads))
	fmt.Printf("  min: %.2f  p50: %.2f  p95: %.2f  max: %.2f\n",
		spreads[0],
		spreads[len(spreads)/2],
		spreads[int(float64(len(spreads)-1)*0.95)],
		spreads[len(spreads)-1],
	)
}
