package reports

import (
	"errors"
	"fmt"
	"math"

	"github.com/theapemachine/symm/research/metrics"
)

/*
ValidationRun is the deterministic input to an offline strategy report.
*/
type ValidationRun struct {
	Name            string
	StartingCapital float64
	Trades          []metrics.Trade
	Predictions     []Prediction
	Ablations       []AblationInput
	Folds           []FoldInput
	BucketCount     int
}

type Prediction struct {
	Confidence float64
	Won        bool
}

type AblationInput struct {
	Name   string
	Trades []metrics.Trade
}

type FoldInput struct {
	Name   string
	Trades []metrics.Trade
}

type ValidationReport struct {
	Name        string
	Baseline    metrics.Summary
	Ablations   []AblationResult
	Calibration []CalibrationBucket
	Folds       []FoldReport
}

type AblationResult struct {
	Name        string
	Performance metrics.Summary
	NetPnLDelta float64
}

type CalibrationBucket struct {
	LowerBound     float64
	UpperBound     float64
	Count          int
	MeanConfidence float64
	HitRate        float64
}

type FoldReport struct {
	Name        string
	Performance metrics.Summary
}

/*
Builder turns deterministic replay outputs into validation reports.
*/
type Builder struct{}

func NewBuilder() *Builder {
	return &Builder{}
}

func (builder *Builder) Build(run ValidationRun) (ValidationReport, error) {
	if builder == nil {
		return ValidationReport{}, errors.New("research report: builder is required")
	}

	if run.Name == "" {
		return ValidationReport{}, errors.New("research report: run name is required")
	}

	calculator, calculatorErr := metrics.NewPerformanceCalculator(run.StartingCapital)

	if calculatorErr != nil {
		return ValidationReport{}, calculatorErr
	}

	baseline, baselineErr := calculator.Summarize(run.Trades)

	if baselineErr != nil {
		return ValidationReport{}, baselineErr
	}

	ablations, ablationErr := builder.buildAblations(calculator, baseline, run.Ablations)

	if ablationErr != nil {
		return ValidationReport{}, ablationErr
	}

	calibration, calibrationErr := builder.buildCalibration(run.Predictions, run.BucketCount)

	if calibrationErr != nil {
		return ValidationReport{}, calibrationErr
	}

	folds, foldErr := builder.buildFolds(calculator, run.Folds)

	if foldErr != nil {
		return ValidationReport{}, foldErr
	}

	return ValidationReport{
		Name:        run.Name,
		Baseline:    baseline,
		Ablations:   ablations,
		Calibration: calibration,
		Folds:       folds,
	}, nil
}

func (builder *Builder) buildAblations(
	calculator *metrics.PerformanceCalculator,
	baseline metrics.Summary,
	ablations []AblationInput,
) ([]AblationResult, error) {
	results := make([]AblationResult, 0, len(ablations))

	for _, ablation := range ablations {
		if ablation.Name == "" {
			return nil, errors.New("research report: ablation name is required")
		}

		performance, performanceErr := calculator.Summarize(ablation.Trades)

		if performanceErr != nil {
			return nil, fmt.Errorf("research report: ablation %q: %w", ablation.Name, performanceErr)
		}

		results = append(results, AblationResult{
			Name:        ablation.Name,
			Performance: performance,
			NetPnLDelta: performance.NetPnL - baseline.NetPnL,
		})
	}

	return results, nil
}

func (builder *Builder) buildCalibration(
	predictions []Prediction,
	bucketCount int,
) ([]CalibrationBucket, error) {
	if len(predictions) == 0 {
		return nil, nil
	}

	if bucketCount <= 0 {
		return nil, errors.New("research report: bucket count must be positive")
	}

	buckets := make([]CalibrationBucket, bucketCount)

	for bucketIndex := range buckets {
		buckets[bucketIndex].LowerBound = float64(bucketIndex) / float64(bucketCount)
		buckets[bucketIndex].UpperBound = float64(bucketIndex+1) / float64(bucketCount)
	}

	wins := make([]int, bucketCount)
	confidenceTotals := make([]float64, bucketCount)

	for _, prediction := range predictions {
		if prediction.Confidence < 0 || prediction.Confidence > 1 {
			return nil, fmt.Errorf(
				"research report: confidence %.4f outside [0,1]",
				prediction.Confidence,
			)
		}

		bucketIndex := int(math.Floor(prediction.Confidence * float64(bucketCount)))

		if bucketIndex >= bucketCount {
			bucketIndex = bucketCount - 1
		}

		buckets[bucketIndex].Count++
		confidenceTotals[bucketIndex] += prediction.Confidence

		if prediction.Won {
			wins[bucketIndex]++
		}
	}

	for bucketIndex := range buckets {
		count := buckets[bucketIndex].Count

		if count == 0 {
			continue
		}

		buckets[bucketIndex].MeanConfidence = confidenceTotals[bucketIndex] / float64(count)
		buckets[bucketIndex].HitRate = float64(wins[bucketIndex]) / float64(count)
	}

	return buckets, nil
}

func (builder *Builder) buildFolds(
	calculator *metrics.PerformanceCalculator,
	folds []FoldInput,
) ([]FoldReport, error) {
	reports := make([]FoldReport, 0, len(folds))

	for _, fold := range folds {
		if fold.Name == "" {
			return nil, errors.New("research report: fold name is required")
		}

		performance, performanceErr := calculator.Summarize(fold.Trades)

		if performanceErr != nil {
			return nil, fmt.Errorf("research report: fold %q: %w", fold.Name, performanceErr)
		}

		reports = append(reports, FoldReport{
			Name:        fold.Name,
			Performance: performance,
		})
	}

	return reports, nil
}
