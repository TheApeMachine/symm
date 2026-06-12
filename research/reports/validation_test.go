package reports

import (
	"testing"
	"time"

	"github.com/theapemachine/symm/research/metrics"
)

func TestBuilderBuildValidationReport(test *testing.T) {
	builder := NewBuilder()
	base := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	trades := []metrics.Trade{
		validationTrade("trade-1", "BTC/USD", base, 100, 111),
		validationTrade("trade-2", "ETH/USD", base.Add(time.Minute), 200, 190),
	}

	report, reportErr := builder.Build(ValidationRun{
		Name:            "baseline",
		StartingCapital: 1000,
		Trades:          trades,
		Predictions: []Prediction{
			{Confidence: 0.2, Won: false},
			{Confidence: 0.8, Won: true},
		},
		Ablations: []AblationInput{{
			Name:   "without_hawkes",
			Trades: trades[:1],
		}},
		Folds: []FoldInput{{
			Name:   "fold-1",
			Trades: trades[:1],
		}},
		BucketCount: 2,
	})

	if reportErr != nil {
		test.Fatalf("report: %v", reportErr)
	}

	if report.Baseline.Trades != 2 {
		test.Fatalf("expected 2 baseline trades, got %d", report.Baseline.Trades)
	}

	if len(report.Ablations) != 1 {
		test.Fatalf("expected one ablation, got %d", len(report.Ablations))
	}

	if report.Ablations[0].NetPnLDelta <= 0 {
		test.Fatalf("expected positive ablation delta, got %f", report.Ablations[0].NetPnLDelta)
	}

	if len(report.Calibration) != 2 {
		test.Fatalf("expected two calibration buckets, got %d", len(report.Calibration))
	}

	if report.Calibration[1].HitRate != 1 {
		test.Fatalf("expected upper bucket hit rate 1, got %f", report.Calibration[1].HitRate)
	}

	if len(report.Folds) != 1 {
		test.Fatalf("expected one fold, got %d", len(report.Folds))
	}
}

func TestBuilderRejectsInvalidConfidence(test *testing.T) {
	_, reportErr := NewBuilder().Build(ValidationRun{
		Name:            "bad-confidence",
		StartingCapital: 1000,
		Predictions: []Prediction{{
			Confidence: 1.2,
			Won:        true,
		}},
		BucketCount: 2,
	})

	if reportErr == nil {
		test.Fatal("expected invalid confidence error")
	}
}

func BenchmarkBuilderBuild(benchmark *testing.B) {
	builder := NewBuilder()
	base := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	trades := make([]metrics.Trade, 0, 64)

	for tradeIndex := 0; tradeIndex < 64; tradeIndex++ {
		trades = append(
			trades,
			validationTrade(
				"bench",
				"BTC/USD",
				base.Add(time.Duration(tradeIndex)*time.Minute),
				100,
				101+float64(tradeIndex%3),
			),
		)
	}

	run := ValidationRun{
		Name:            "bench",
		StartingCapital: 1000,
		Trades:          trades,
		Predictions: []Prediction{
			{Confidence: 0.25, Won: false},
			{Confidence: 0.75, Won: true},
		},
		Ablations: []AblationInput{{
			Name:   "without_signal",
			Trades: trades[:32],
		}},
		Folds: []FoldInput{{
			Name:   "fold-1",
			Trades: trades[:32],
		}},
		BucketCount: 4,
	}

	for benchmark.Loop() {
		if _, reportErr := builder.Build(run); reportErr != nil {
			benchmark.Fatal(reportErr)
		}
	}
}

func validationTrade(
	id string,
	symbol string,
	entryAt time.Time,
	entryNotional float64,
	exitNotional float64,
) metrics.Trade {
	return metrics.Trade{
		ID:            id,
		Symbol:        symbol,
		EntryAt:       entryAt,
		ExitAt:        entryAt.Add(30 * time.Second),
		EntryNotional: entryNotional,
		ExitNotional:  exitNotional,
		Fees:          1,
	}
}
