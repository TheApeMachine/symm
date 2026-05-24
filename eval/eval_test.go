package eval

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/engine"
)

func TestCollectorRecord(t *testing.T) {
	collector := NewCollector()
	now := time.Unix(1_700_000_000, 0)

	collector.Record(engine.PredictionFeedback{
		Source:          "hawkes",
		Symbol:          "PUMP/EUR",
		PredictedReturn: 0.002,
		ActualReturn:    0.001,
		Error:           0.001,
		Confidence:      0.8,
		SettledAt:       now,
	})

	collector.Record(engine.PredictionFeedback{
		Source:     "hawkes",
		Symbol:     "PUMP/EUR",
		Unanchored: true,
	})

	records := collector.Records()

	if len(records) != 1 {
		t.Fatalf("expected one settled record, got %d", len(records))
	}

	if !records[0].Hit {
		t.Fatal("expected positive actual return to count as hit")
	}
}

func TestBuildReportSummaries(t *testing.T) {
	records := []Record{
		{Signal: "hawkes", Symbol: "PUMP/EUR", Confidence: 0.2, PredictedReturn: 0.002, ActualReturn: 0.001, Error: 0.001, Hit: true},
		{Signal: "hawkes", Symbol: "PUMP/EUR", Confidence: 0.8, PredictedReturn: 0.004, ActualReturn: 0.003, Error: 0.001, Hit: true},
		{Signal: "fluid", Symbol: "BTC/EUR", Confidence: 0.5, PredictedReturn: 0.001, ActualReturn: -0.001, Error: 0.002, Hit: false},
	}

	report := BuildReport("fixture.jsonl", records)

	if len(report.Sources) != 2 {
		t.Fatalf("expected two source rows, got %d", len(report.Sources))
	}

	if len(report.Deciles) == 0 {
		t.Fatal("expected decile rows")
	}
}

func TestWriteJSONAndCSV(t *testing.T) {
	report := BuildReport("fixture.jsonl", []Record{
		{Signal: "hawkes", Symbol: "PUMP/EUR", Confidence: 0.5, PredictedReturn: 0.002, ActualReturn: 0.001, Error: 0.001, Hit: true},
	})

	jsonBuffer := bytes.NewBuffer(nil)

	if err := WriteJSON(jsonBuffer, report); err != nil {
		t.Fatalf("write json: %v", err)
	}

	csvBuffer := bytes.NewBuffer(nil)

	if err := WriteCSV(csvBuffer, report); err != nil {
		t.Fatalf("write csv: %v", err)
	}

	if jsonBuffer.Len() == 0 || csvBuffer.Len() == 0 {
		t.Fatal("expected non-empty export buffers")
	}
}

func TestRunReplayFixture(t *testing.T) {
	convey.Convey("Given the sample replay fixture", t, func() {
		report, err := Run(context.Background(), Options{
			ReplayFile: "../replay/fixtures/sample.jsonl",
			MaxTicks:   128,
		})

		convey.Convey("It should produce a calibration report without error", func() {
			convey.So(err, convey.ShouldBeNil)
			convey.So(report.ReplayFile, convey.ShouldEqual, "../replay/fixtures/sample.jsonl")
		})
	})
}

func BenchmarkBuildReport(b *testing.B) {
	records := make([]Record, 0, 256)

	for index := 0; index < 256; index++ {
		records = append(records, Record{
			Signal:          "hawkes",
			Symbol:          "PUMP/EUR",
			Confidence:      float64(index) / 256,
			PredictedReturn: 0.002,
			ActualReturn:    0.001,
			Error:           0.001,
			Hit:             true,
		})
	}

	b.ReportAllocs()

	for b.Loop() {
		_ = BuildReport("bench.jsonl", records)
	}
}
