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
	ready bool,
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
	validity := types.MeasurementValidity{
		State:     types.ValidityValid,
		Readiness: types.ReadinessObservation,
	}

	if !ready {
		validity.State = types.ValidityProvisional
		validity.Reason = "ignition baselines not ready"
	}
	scale := types.ScaleReference{
		Kind:    types.ScaleObservationWindow,
		From:    at,
		Through: at,
	}

	return []*types.Measurement{
		{
			Source:     types.SourcePumpDump,
			Stream:     types.PumpDump,
			Metric:     types.MetricRVOL,
			Subject:    types.SubjectPumpVolumeLift,
			Symbol:     symbol,
			At:         at,
			Unit:       types.UnitDimensionless,
			Raw:        output.RVOL,
			Normalized: types.NormalizeFinite(output.RVOL),
			Maturity:   maturity,
			Validity:   validity,
			Scale:      scale,
		},
		{
			Source:     types.SourcePumpDump,
			Stream:     types.PumpDump,
			Metric:     types.MetricPrecursor,
			Subject:    types.SubjectPumpPriceLift,
			Symbol:     symbol,
			At:         at,
			Unit:       types.UnitDimensionless,
			Raw:        output.Precursor,
			Normalized: types.NormalizeFinite(output.Precursor),
			Maturity:   maturity,
			Validity:   validity,
			Scale:      scale,
		},
		{
			Source:     types.SourcePumpDump,
			Stream:     types.PumpDump,
			Metric:     types.MetricSpread,
			Subject:    types.SubjectPumpSpread,
			Symbol:     symbol,
			At:         at,
			Unit:       types.UnitQuoteCurrency,
			Raw:        output.Spread,
			Normalized: types.NormalizeRatio(output.Spread, mid),
			Maturity:   maturity,
			Validity:   validity,
			Scale:      scale,
		},
		{
			Source:     types.SourcePumpDump,
			Stream:     types.PumpDump,
			Metric:     types.MetricCompression,
			Subject:    types.SubjectPumpCompression,
			Symbol:     symbol,
			At:         at,
			Unit:       types.UnitDimensionless,
			Raw:        output.Compression,
			Normalized: types.NormalizeFinite(output.Compression),
			Maturity:   maturity,
			Validity:   validity,
			Scale:      scale,
		},
		{
			Source:     types.SourcePumpDump,
			Stream:     types.PumpDump,
			Metric:     types.MetricIgnition,
			Subject:    types.SubjectPumpIgnition,
			Symbol:     symbol,
			At:         at,
			Unit:       types.UnitDimensionless,
			Raw:        output.Ignition,
			Normalized: types.NormalizeFinite(output.Ignition),
			Maturity:   maturity,
			Validity:   validity,
			Scale:      scale,
		},
		{
			Source:     types.SourcePumpDump,
			Stream:     types.PumpDump,
			Metric:     types.MetricTrend,
			Subject:    types.SubjectPumpTrend,
			Symbol:     symbol,
			At:         at,
			Unit:       types.UnitDimensionless,
			Raw:        output.Trend,
			Normalized: types.NormalizeFinite(output.Trend),
			Maturity:   maturity,
			Validity:   validity,
			Scale:      scale,
		},
		{
			Source:     types.SourcePumpDump,
			Stream:     types.PumpDump,
			Metric:     types.MetricExhaustion,
			Subject:    types.SubjectPumpExhaustion,
			Symbol:     symbol,
			At:         at,
			Unit:       types.UnitDimensionless,
			Raw:        output.Exhaustion,
			Normalized: types.NormalizeFinite(output.Exhaustion),
			Maturity:   maturity,
			Validity:   validity,
			Scale:      scale,
		},
		{
			Source:     types.SourcePumpDump,
			Stream:     types.PumpDump,
			Metric:     types.MetricStrength,
			Subject:    types.SubjectPumpComposite,
			Symbol:     symbol,
			At:         at,
			Unit:       types.UnitDimensionless,
			Raw:        output.Strength,
			Normalized: types.NormalizeFinite(output.Strength),
			Maturity:   maturity,
			Validity:   validity,
			Scale:      scale,
		},
	}, nil
}
