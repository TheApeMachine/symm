package pumpdump

import (
	"fmt"
	"time"

	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/symm/types"
)

func ignitionMeasurements(
	symbol string,
	at time.Time,
	output equation.IgnitionOutput,
	maturity float64,
	bid float64,
	ask float64,
) ([]*types.Measurement, error) {
	if at.IsZero() {
		return nil, fmt.Errorf("pumpdump: observation timestamp required")
	}

	if bid <= 0 || ask <= 0 {
		return nil, fmt.Errorf("pumpdump: bid and ask must be positive")
	}

	if bid >= ask {
		return nil, fmt.Errorf("pumpdump: crossed BBO bid=%v ask=%v", bid, ask)
	}

	mid := (bid + ask) / 2

	return []*types.Measurement{
		types.ObservationMeasurement(
			types.SourcePumpDump, types.PumpDump, types.MetricRVOL,
			types.SubjectPumpVolumeLift, symbol, at,
			types.UnitDimensionless, output.RVOL, maturity,
		),
		types.ObservationMeasurement(
			types.SourcePumpDump, types.PumpDump, types.MetricPrecursor,
			types.SubjectPumpPriceLift, symbol, at,
			types.UnitDimensionless, output.Precursor, maturity,
		),
		types.ObservationNormalizedMeasurement(
			types.SourcePumpDump, types.PumpDump, types.MetricSpread,
			types.SubjectPumpSpread, symbol, at,
			types.UnitQuoteCurrency, output.Spread, maturity,
			types.NormalizeRatio(output.Spread, mid),
		),
		types.ObservationMeasurement(
			types.SourcePumpDump, types.PumpDump, types.MetricCompression,
			types.SubjectPumpCompression, symbol, at,
			types.UnitDimensionless, output.Compression, maturity,
		),
		types.ObservationMeasurement(
			types.SourcePumpDump, types.PumpDump, types.MetricIgnition,
			types.SubjectPumpIgnition, symbol, at,
			types.UnitDimensionless, output.Ignition, maturity,
		),
		types.ObservationMeasurement(
			types.SourcePumpDump, types.PumpDump, types.MetricTrend,
			types.SubjectPumpTrend, symbol, at,
			types.UnitDimensionless, output.Trend, maturity,
		),
		types.ObservationMeasurement(
			types.SourcePumpDump, types.PumpDump, types.MetricExhaustion,
			types.SubjectPumpExhaustion, symbol, at,
			types.UnitDimensionless, output.Exhaustion, maturity,
		),
		types.ObservationMeasurement(
			types.SourcePumpDump, types.PumpDump, types.MetricStrength,
			types.SubjectPumpComposite, symbol, at,
			types.UnitDimensionless, output.Strength, maturity,
		),
	}, nil
}
