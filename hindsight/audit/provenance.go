package audit

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique/data"
)

/*
FeeProvenance checks the declared taker fee against the stored detections.
Detections do not record the fee detect ran with, but they bound it: a move
low -> high clears round-trip friction at fee f exactly when
high*(1-f) > low*(1+f), i.e. f < (high-low)/(high+low). Every up/down leg
cleared, so the detect fee is below the smallest such ratio (Upper); every
up_friction near miss failed, so it is at least the largest (Lower). A
declared fee outside [Lower, Upper) cannot be the one the detections were
made at. Inside the interval is consistent, not proven.
*/
type FeeProvenance struct {
	DeclaredFee float64 `json:"declared_fee"`
	Lower       float64 `json:"lower"`
	// Upper is nil when no up/down leg bounds the fee from above.
	Upper       *float64 `json:"upper"`
	Bounding    int      `json:"bounding_detections"`
	Consistent  bool     `json:"consistent"`
	SummaryText string   `json:"summary_text"`
}

func breakEvenFee(low, high float64) float64 {
	return (high - low) / (high + low)
}

/*
checkFeeProvenance bounds the detect fee from detections and compares it
with declared.
*/
func checkFeeProvenance(detections []*data.Measurement, declared float64) FeeProvenance {
	report := FeeProvenance{DeclaredFee: declared}
	upper := math.Inf(1)

	for _, det := range detections {
		if det == nil {
			continue
		}

		b, c := getMeasurementMetric(det, "b_price"), getMeasurementMetric(det, "c_price")

		if b <= 0 || c <= 0 {
			continue
		}

		low, high := math.Min(b, c), math.Max(b, c)

		switch det.Meta("type") {
		case "up", "down":
			upper = math.Min(upper, breakEvenFee(low, high))
			report.Bounding++
		case "up_friction":
			report.Lower = math.Max(report.Lower, breakEvenFee(low, high))
			report.Bounding++
		}
	}

	if !math.IsInf(upper, 1) {
		report.Upper = &upper
	}

	report.Consistent = report.Bounding > 0 && declared >= report.Lower && declared < upper
	report.SummaryText = fmt.Sprintf(
		"declared fee %.5f; stored detections bound the detect fee to [%.5f, %.5f) from %d legs; consistent=%t",
		declared, report.Lower, upper, report.Bounding, report.Consistent,
	)

	return report
}
