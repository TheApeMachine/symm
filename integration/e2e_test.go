package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/internal/testconfig"
)

func TestIntegrationE2E(t *testing.T) {
	Convey("Given the replay-backed integration harness", t, func() {
		testconfig.Load(t)
		resetTradingReady()

		auditPath := filepath.Join(t.TempDir(), "audit.jsonl")
		ConfigureViper(auditPath)

		reportPath := filepath.Join("runs", "e2e-report.json")
		report := RunSuite(context.Background(), defaultScenarios())

		_ = os.MkdirAll(filepath.Dir(reportPath), 0o755)
		writeErr := report.WriteJSON(reportPath)

		Convey("It should write a JSON report artifact", func() {
			So(writeErr, ShouldBeNil)

			_, statErr := os.Stat(reportPath)
			So(statErr, ShouldBeNil)
		})

		Convey("It should print a granular pass/fail report", func() {
			t.Log("\n" + report.FormatText())
		})

		Convey("Every scenario check should pass", func() {
			So(report.Pass, ShouldBeTrue)
		})
	})
}

/*
RunSuite executes all scenarios and returns an aggregate report.
*/
func RunSuite(parent context.Context, scenarios []Scenario) *SuiteReport {
	started := time.Now()
	suite := &SuiteReport{
		StartedAt: started,
		Scenarios: make([]ScenarioReport, 0, len(scenarios)),
	}

	for _, scenario := range scenarios {
		builder := buildCapture(scenario)
		ctx, cancel := context.WithCancel(parent)

		harness, err := NewHarness(ctx, builder.Reader())

		if err != nil {
			cancel()
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
		cancel()
		resetTradingReady()
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
		harness, err := NewHarness(ctx, builder.Reader())

		if err != nil {
			b.Fatal(err)
		}

		_ = harness.RunScenario(Scenario{
			ID:   "bench.replay",
			Name: "Replay baseline",
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
		resetTradingReady()
	}
}
