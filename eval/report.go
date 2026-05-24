package eval

import (
	"math"
	"sort"

	"github.com/theapemachine/symm/stats"
)

const decileCount = 10

/*
SourceRow summarizes calibration for one signal and symbol pair.
*/
type SourceRow struct {
	Signal                string  `json:"signal"`
	Symbol                string  `json:"symbol"`
	Count                 int     `json:"count"`
	AvgPredictedReturn    float64 `json:"avg_predicted_return"`
	AvgActualReturn       float64 `json:"avg_actual_return"`
	CalibrationRatio      float64 `json:"calibration_ratio"`
	HitRate               float64 `json:"hit_rate"`
	MeanError             float64 `json:"mean_error"`
	MedianError           float64 `json:"median_error"`
	P95Error              float64 `json:"p95_error"`
	MaxDrawdownAfterEntry float64 `json:"max_drawdown_after_entry"`
}

/*
DecileRow summarizes forward returns for one signal confidence decile.
*/
type DecileRow struct {
	Signal           string  `json:"signal"`
	ConfidenceDecile int     `json:"confidence_decile"`
	Count            int     `json:"count"`
	AvgConfidence    float64 `json:"avg_confidence"`
	AvgForwardReturn float64 `json:"avg_forward_return"`
}

/*
Report is the offline replay calibration output.
*/
type Report struct {
	ReplayFile string      `json:"replay_file"`
	Records    int         `json:"records"`
	Sources    []SourceRow `json:"sources"`
	Deciles    []DecileRow `json:"deciles"`
}

/*
BuildReport aggregates collector records into source and decile summaries.
*/
func BuildReport(replayFile string, records []Record) Report {
	report := Report{
		ReplayFile: replayFile,
		Records:    len(records),
		Sources:    buildSourceRows(records),
		Deciles:    buildDecileRows(records),
	}

	return report
}

func buildSourceRows(records []Record) []SourceRow {
	type key struct {
		signal string
		symbol string
	}

	buckets := make(map[key][]Record)

	for _, record := range records {
		bucketKey := key{signal: record.Signal, symbol: record.Symbol}
		buckets[bucketKey] = append(buckets[bucketKey], record)
	}

	rows := make([]SourceRow, 0, len(buckets))

	for bucketKey, bucket := range buckets {
		rows = append(rows, summarizeSource(bucketKey.signal, bucketKey.symbol, bucket))
	}

	sort.Slice(rows, func(left, right int) bool {
		if rows[left].Signal != rows[right].Signal {
			return rows[left].Signal < rows[right].Signal
		}

		return rows[left].Symbol < rows[right].Symbol
	})

	return rows
}

func summarizeSource(signal, symbol string, records []Record) SourceRow {
	predictedSum := 0.0
	actualSum := 0.0
	errorSum := 0.0
	errorValues := make([]float64, 0, len(records))
	hits := 0
	maxDrawdown := 0.0

	for _, record := range records {
		predictedSum += record.PredictedReturn
		actualSum += record.ActualReturn
		errorSum += math.Abs(record.Error)
		errorValues = append(errorValues, math.Abs(record.Error))

		if record.Hit {
			hits++
		}

		if record.ActualReturn < maxDrawdown {
			maxDrawdown = record.ActualReturn
		}
	}

	count := len(records)
	avgPredicted := predictedSum / float64(count)
	avgActual := actualSum / float64(count)
	calibration := 0.0

	if avgPredicted != 0 {
		calibration = avgActual / avgPredicted
	}

	sortedErrors := stats.CopySorted(errorValues)

	return SourceRow{
		Signal:                signal,
		Symbol:                symbol,
		Count:                 count,
		AvgPredictedReturn:    avgPredicted,
		AvgActualReturn:       avgActual,
		CalibrationRatio:      calibration,
		HitRate:               float64(hits) / float64(count),
		MeanError:             errorSum / float64(count),
		MedianError:           stats.PercentileSorted(sortedErrors, 0.5),
		P95Error:              stats.PercentileSorted(sortedErrors, 0.95),
		MaxDrawdownAfterEntry: maxDrawdown,
	}
}

func buildDecileRows(records []Record) []DecileRow {
	bySignal := make(map[string][]Record)

	for _, record := range records {
		bySignal[record.Signal] = append(bySignal[record.Signal], record)
	}

	rows := make([]DecileRow, 0, len(bySignal)*decileCount)

	for signal, bucket := range bySignal {
		rows = append(rows, summarizeDeciles(signal, bucket)...)
	}

	sort.Slice(rows, func(left, right int) bool {
		if rows[left].Signal != rows[right].Signal {
			return rows[left].Signal < rows[right].Signal
		}

		return rows[left].ConfidenceDecile < rows[right].ConfidenceDecile
	})

	return rows
}

func summarizeDeciles(signal string, records []Record) []DecileRow {
	sorted := append([]Record(nil), records...)

	sort.Slice(sorted, func(left, right int) bool {
		if sorted[left].Confidence != sorted[right].Confidence {
			return sorted[left].Confidence < sorted[right].Confidence
		}

		return sorted[left].SettledAt.Before(sorted[right].SettledAt)
	})

	rows := make([]DecileRow, 0, decileCount)

	for decile := 1; decile <= decileCount; decile++ {
		start := (decile - 1) * len(sorted) / decileCount
		end := decile * len(sorted) / decileCount

		if start >= end {
			continue
		}

		slice := sorted[start:end]
		confidenceSum := 0.0
		returnSum := 0.0

		for _, record := range slice {
			confidenceSum += record.Confidence
			returnSum += record.ActualReturn
		}

		rows = append(rows, DecileRow{
			Signal:           signal,
			ConfidenceDecile: decile,
			Count:            len(slice),
			AvgConfidence:    confidenceSum / float64(len(slice)),
			AvgForwardReturn: returnSum / float64(len(slice)),
		})
	}

	return rows
}
