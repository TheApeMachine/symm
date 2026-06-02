package integration

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

/*
CheckResult is one granular pass/fail assertion with debugging context.
*/
type CheckResult struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Pass    bool   `json:"pass"`
	Detail  string `json:"detail,omitempty"`
	Context map[string]any `json:"context,omitempty"`
}

/*
ScenarioReport summarizes one synthetic scenario run.
*/
type ScenarioReport struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Pass      bool           `json:"pass"`
	StartedAt time.Time      `json:"started_at"`
	Elapsed   time.Duration  `json:"elapsed"`
	Checks    []CheckResult  `json:"checks"`
}

/*
SuiteReport is the full end-to-end integration run.
*/
type SuiteReport struct {
	Pass      bool             `json:"pass"`
	StartedAt time.Time        `json:"started_at"`
	Elapsed   time.Duration    `json:"elapsed"`
	Scenarios []ScenarioReport `json:"scenarios"`
}

func (report *SuiteReport) finalize() {
	report.Pass = true

	for _, scenario := range report.Scenarios {
		if !scenario.Pass {
			report.Pass = false
		}
	}
}

/*
FormatText renders a human-readable pass/fail report.
*/
func (report *SuiteReport) FormatText() string {
	var builder strings.Builder

	status := "PASS"

	if !report.Pass {
		status = "FAIL"
	}

	fmt.Fprintf(
		&builder,
		"integration e2e: %s (%d scenarios, %s)\n",
		status,
		len(report.Scenarios),
		report.Elapsed.Round(time.Millisecond),
	)

	for _, scenario := range report.Scenarios {
		scenarioStatus := "PASS"

		if !scenario.Pass {
			scenarioStatus = "FAIL"
		}

		fmt.Fprintf(
			&builder,
			"\n[%s] %s — %s (%s)\n",
			scenarioStatus,
			scenario.ID,
			scenario.Name,
			scenario.Elapsed.Round(time.Millisecond),
		)

		for _, check := range scenario.Checks {
			checkStatus := "pass"

			if !check.Pass {
				checkStatus = "FAIL"
			}

			fmt.Fprintf(&builder, "  • %s [%s] %s\n", checkStatus, check.ID, check.Name)

			if check.Detail != "" {
				fmt.Fprintf(&builder, "      %s\n", check.Detail)
			}

			if len(check.Context) > 0 {
				raw, err := json.Marshal(check.Context)

				if err == nil {
					fmt.Fprintf(&builder, "      context: %s\n", string(raw))
				}
			}
		}
	}

	return builder.String()
}

/*
WriteJSON persists the suite report.
*/
func (report *SuiteReport) WriteJSON(path string) error {
	raw, err := json.MarshalIndent(report, "", "  ")

	if err != nil {
		return fmt.Errorf("integration report: marshal: %w", err)
	}

	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return fmt.Errorf("integration report: write %q: %w", path, err)
	}

	return nil
}

func writeReport(writer io.Writer, report *SuiteReport) {
	if writer == nil {
		return
	}

	_, _ = io.WriteString(writer, report.FormatText())
}
