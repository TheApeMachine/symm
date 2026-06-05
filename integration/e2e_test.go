package integration

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/internal/testconfig"
)

const (
	integrationE2ESuiteTimeout = 330 * time.Second
	// integrationE2EMinTestTimeout is the go test -timeout floor (per-category signal validation).
	integrationE2EMinTestTimeout = 300 * time.Second
)

func TestIntegrationE2E(t *testing.T) {
	if deadline, hasDeadline := t.Deadline(); hasDeadline {
		remaining := time.Until(deadline)

		if remaining < integrationE2EMinTestTimeout {
			t.Fatalf(
				"TestIntegrationE2E needs go test -timeout at least %s (have %s); run: make test-e2e",
				integrationE2EMinTestTimeout,
				remaining.Round(time.Second),
			)
		}
	}

	Convey("Given the replay-backed integration harness", t, func() {
		testconfig.Load(t)

		auditDir := t.TempDir()
		ConfigureViper(filepath.Join(auditDir, "suite-audit.jsonl"))

		reportPath := filepath.Join("runs", "e2e-report.json")
		suiteCtx, suiteCancel := context.WithTimeout(context.Background(), integrationE2ESuiteTimeout)
		defer suiteCancel()

		availableScenarios := allScenarios()
		scenarios := selectedScenarios(availableScenarios)

		if len(scenarios) == 0 {
			t.Fatalf(
				"SYMM_E2E_SCENARIO=%q did not match any scenario; available: %s",
				os.Getenv("SYMM_E2E_SCENARIO"),
				strings.Join(scenarioIDs(availableScenarios), ", "),
			)
		}

		report := RunSuite(suiteCtx, auditDir, scenarios)

		_ = os.MkdirAll(filepath.Dir(reportPath), 0o755)
		writeErr := report.WriteJSON(reportPath)

		Convey("It should write the report and pass every scenario", func() {
			So(writeErr, ShouldBeNil)

			_, statErr := os.Stat(reportPath)
			So(statErr, ShouldBeNil)

			t.Log("\n" + report.FormatText())

			So(report.Pass, ShouldBeTrue)
		})
	})
}

func selectedScenarios(scenarios []Scenario) []Scenario {
	filter := os.Getenv("SYMM_E2E_SCENARIO")

	if filter == "" {
		return scenarios
	}

	selected := make([]Scenario, 0, 1)

	for _, scenario := range scenarios {
		if scenario.ID == filter {
			selected = append(selected, scenario)
		}
	}

	return selected
}

func scenarioIDs(scenarios []Scenario) []string {
	ids := make([]string, 0, len(scenarios))

	for _, scenario := range scenarios {
		ids = append(ids, scenario.ID)
	}

	return ids
}

/*
RunSuite executes all scenarios and returns an aggregate report.
*/
func RunSuite(parent context.Context, auditDir string, scenarios []Scenario) *SuiteReport {
	started := time.Now()
	suite := &SuiteReport{
		StartedAt: started,
		Scenarios: make([]ScenarioReport, 0, len(scenarios)),
	}

	for _, scenario := range scenarios {
		scenarioAudit := filepath.Join(auditDir, scenario.ID+".jsonl")
		ConfigureViper(scenarioAudit)

		builder := buildCapture(scenario)

		runTimeout := scenario.RunTimeout

		if runTimeout <= 0 {
			runTimeout = 6 * time.Second
		}

		scenarioCtx, scenarioCancel := context.WithTimeout(parent, runTimeout)

		harness, err := NewHarness(scenarioCtx, builder.Reader(), scenarioAudit)

		if err != nil {
			scenarioCancel()
			suite.Scenarios = append(suite.Scenarios, ScenarioReport{
				ID:   scenario.ID,
				Name: scenario.Name,
				Pass: false,
				Checks: []CheckResult{{
					ID:     "harness.boot",
					Name:   "Harness construction",
					Pass:   false,
					Detail: err.Error(),
				}},
			})

			continue
		}

		suite.Scenarios = append(suite.Scenarios, harness.RunScenario(scenario))
		harness.Close()
		scenarioCancel()
	}

	suite.Elapsed = time.Since(started)
	suite.finalize()

	return suite
}

func BenchmarkIntegrationReplayHarness(b *testing.B) {
	testconfig.MustLoad()
	ConfigureViper(filepath.Join(b.TempDir(), "audit.jsonl"))

	builder := NewCaptureBuilder(time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC))
	builder.AppendBaselineMarket()

	for b.Loop() {
		ctx, cancel := context.WithCancel(context.Background())
		harness, err := NewHarness(ctx, builder.Reader(), filepath.Join(b.TempDir(), "audit.jsonl"))

		if err != nil {
			b.Fatal(err)
		}

		_ = harness.RunScenario(Scenario{
			ID:          "bench.replay",
			Name:        "Replay baseline",
			SettleDelay: 200 * time.Millisecond,
			Checks: []ScenarioCheck{{
				ID:   "raw.frames",
				Name: "raw frames",
				Evaluate: func(snapshot TapeSnapshot, _ error) (bool, string, map[string]any) {
					return snapshot.RawFrames > 0, "", nil
				},
			}},
		})
		harness.Close()
		cancel()
	}
}
