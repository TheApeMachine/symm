package integration

import (
	"bufio"
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/internal/testconfig"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/public"
)

func TestDecodeCaptureFrame(t *testing.T) {
	Convey("Given a synthetic capture frame from CaptureBuilder", t, func() {
		builder := NewCaptureBuilder(time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC))
		builder.AppendInstrumentCatalog()

		scanner := bufio.NewScanner(builder.Reader())
		So(scanner.Scan(), ShouldBeTrue)

		frame, err := decodeCaptureFrame(scanner.Bytes())

		Convey("It should preserve channel metadata and JSON payload", func() {
			So(err, ShouldBeNil)
			So(frame.Channel, ShouldEqual, public.InstrumentsChannel)
			So(len(frame.Data), ShouldBeGreaterThan, 0)

			update := market.InstrumentUpdate{}
			So(sonic.Unmarshal(frame.Data, &update), ShouldBeNil)
			So(len(update.Pairs), ShouldBeGreaterThan, 0)
		})
	})
}

func TestHarnessRunScenario(t *testing.T) {
	Convey("Given a replay-backed harness scenario", t, func() {
		testconfig.Load(t)

		auditPath := filepath.Join(t.TempDir(), "audit.jsonl")
		ConfigureViper(auditPath)

		builder := NewCaptureBuilder(time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC))
		builder.AppendBaselineMarket()

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		harness, err := NewHarness(ctx, builder.Reader(), auditPath)
		So(err, ShouldBeNil)
		defer harness.Close()

		report := harness.RunScenario(Scenario{
			ID:          "harness.shutdown",
			Name:        "Harness shutdown",
			SettleDelay: 20 * time.Millisecond,
			Checks: []ScenarioCheck{{
				ID:   "raw.frames",
				Name: "raw frames",
				Evaluate: func(snapshot TapeSnapshot, _ error) (bool, string, map[string]any) {
					return snapshot.RawFrames > 0, "", nil
				},
			}},
		})

		Convey("It should replay and stop the engine before the context deadline", func() {
			So(report.Pass, ShouldBeTrue)
			So(ctx.Err(), ShouldBeNil)
		})
	})
}

func TestHarnessRunScenarioLimitFill(t *testing.T) {
	Convey("Given the limit-fill execution scenario", t, func() {
		testconfig.Load(t)

		auditPath := filepath.Join(t.TempDir(), "audit.jsonl")
		ConfigureViper(auditPath)

		scenarios := executionScenarios()

		if len(scenarios) == 0 {
			t.Fatal("executionScenarios returned no scenarios")
		}

		scenario := scenarios[0]
		builder := buildCapture(scenario)
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		harness, err := NewHarness(ctx, builder.Reader(), auditPath)
		So(err, ShouldBeNil)
		defer harness.Close()

		report := harness.RunScenario(scenario)

		Convey("It should publish the action, fill, inventory, and wallet change", func() {
			So(report.Pass, ShouldBeTrue)
		})
	})
}
