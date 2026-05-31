package perspectives

import (
	"fmt"
	"strings"

	"github.com/theapemachine/symm/numeric/adaptive"
)

var playbookScoreFloors = adaptive.NewSNRField()

/*
ScoreSeriesKey identifies one adaptive SNR baseline: source, category, optional
stream (e.g. depthflow imbalance vs level1), and symbol. All playbook-facing
scores are comparable sigma units above that series' own history.
*/
func ScoreSeriesKey(
	source SourceType,
	category CategoryType,
	stream string,
	symbol string,
) string {
	streamLabel := strings.TrimSpace(stream)

	if streamLabel == "" {
		streamLabel = "-"
	}

	symbolLabel := strings.TrimSpace(symbol)

	if symbolLabel == "" {
		symbolLabel = "-"
	}

	return strings.Join([]string{
		source.String(),
		category.String(),
		"stream:" + streamLabel,
		"symbol:" + symbolLabel,
	}, ":")
}

/*
FinalizeMeasurement stores raw fused strength for gauges and sets SNR from the
shared adaptive floor for this series. SNR stays 0 while the floor is warming up;
callers must not treat SNR as a gate until it is positive.
*/
func FinalizeMeasurement(
	measurement Measurement,
	raw float64,
	stream string,
) Measurement {
	measurement.Strength = raw
	key := ScoreSeriesKey(
		measurement.Source,
		measurement.Category,
		stream,
		measurement.Symbol,
	)
	measurement.SNR = playbookScoreFloors.Score(key, raw)

	return measurement
}

/*
ResetPlaybookScoreFloors clears adaptive baselines (tests only).
*/
func ResetPlaybookScoreFloors() {
	playbookScoreFloors = adaptive.NewSNRField()
}

/*
FormatScoreSeries documents a scorer key (tests, diagnostics).
*/
func FormatScoreSeries(
	source SourceType,
	category CategoryType,
	stream string,
	symbol string,
) string {
	return fmt.Sprintf("%q", ScoreSeriesKey(source, category, stream, symbol))
}
