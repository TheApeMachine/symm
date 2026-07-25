package pumpdump

import (
	"fmt"
	"time"

	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/symm/types"
)

/*
ignitionMeasurements emits one PumpDump Measurement per symbol whose Metrics
map carries the full ignition surface (RVOL through Strength).
*/
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
	measurement := &types.Measurement{
		Source:   types.SourcePumpDump,
		Symbol:   symbol,
		At:       at,
		Maturity: maturity,
		Validity: validity,
		Scale:    scale,
	}
	specs := []struct {
		metric     types.MetricType
		unit       types.MeasurementUnit
		raw        float64
		normalized *float64
	}{
		{
			types.MetricRVOL, types.UnitDimensionless, output.RVOL,
			types.NormalizeFinite(output.RVOL),
		},
		{
			types.MetricPrecursor, types.UnitDimensionless, output.Precursor,
			types.NormalizeFinite(output.Precursor),
		},
		{
			types.MetricSpread, types.UnitQuoteCurrency, output.Spread,
			types.NormalizeRatio(output.Spread, mid),
		},
		{
			types.MetricCompression, types.UnitDimensionless, output.Compression,
			types.NormalizeFinite(output.Compression),
		},
		{
			types.MetricIgnition, types.UnitDimensionless, output.Ignition,
			types.NormalizeFinite(output.Ignition),
		},
		{
			types.MetricTrend, types.UnitDimensionless, output.Trend,
			types.NormalizeFinite(output.Trend),
		},
		{
			types.MetricExhaustion, types.UnitDimensionless, output.Exhaustion,
			types.NormalizeFinite(output.Exhaustion),
		},
		{
			types.MetricStrength, types.UnitDimensionless, output.Strength,
			types.NormalizeFinite(output.Strength),
		},
	}

	for _, spec := range specs {
		measurement.PutMetric(spec.metric, types.SideNone, types.MetricSample{
			Raw:        spec.raw,
			Normalized: spec.normalized,
			Unit:       spec.unit,
		})
	}

	return []*types.Measurement{measurement}, nil
}
