package eval

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
)

/*
WriteJSON encodes one calibration report.
*/
func WriteJSON(writer io.Writer, report Report) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(report); err != nil {
		return fmt.Errorf("encode eval report: %w", err)
	}

	return nil
}

/*
WriteCSV writes source and decile rows as a flat CSV table.
*/
func WriteCSV(writer io.Writer, report Report) error {
	csvWriter := csv.NewWriter(writer)

	if err := csvWriter.Write([]string{
		"section",
		"signal",
		"symbol",
		"confidence_decile",
		"count",
		"avg_predicted_return",
		"avg_actual_return",
		"calibration_ratio",
		"hit_rate",
		"mean_error",
		"median_error",
		"p95_error",
		"max_drawdown_after_entry",
		"avg_confidence",
		"avg_forward_return",
	}); err != nil {
		return fmt.Errorf("write csv header: %w", err)
	}

	for _, row := range report.Sources {
		if err := csvWriter.Write(sourceCSVRow(row)); err != nil {
			return fmt.Errorf("write source row: %w", err)
		}
	}

	for _, row := range report.Deciles {
		if err := csvWriter.Write(decileCSVRow(row)); err != nil {
			return fmt.Errorf("write decile row: %w", err)
		}
	}

	csvWriter.Flush()

	if err := csvWriter.Error(); err != nil {
		return fmt.Errorf("flush csv: %w", err)
	}

	return nil
}

func sourceCSVRow(row SourceRow) []string {
	return []string{
		"source",
		row.Signal,
		row.Symbol,
		"",
		strconv.Itoa(row.Count),
		formatFloat(row.AvgPredictedReturn),
		formatFloat(row.AvgActualReturn),
		formatFloat(row.CalibrationRatio),
		formatFloat(row.HitRate),
		formatFloat(row.MeanError),
		formatFloat(row.MedianError),
		formatFloat(row.P95Error),
		formatFloat(row.MaxDrawdownAfterEntry),
		"",
		"",
	}
}

func decileCSVRow(row DecileRow) []string {
	return []string{
		"decile",
		row.Signal,
		"",
		strconv.Itoa(row.ConfidenceDecile),
		strconv.Itoa(row.Count),
		"",
		"",
		"",
		"",
		"",
		"",
		"",
		"",
		formatFloat(row.AvgConfidence),
		formatFloat(row.AvgForwardReturn),
	}
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', 6, 64)
}
