package pumpdump

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/market/perspectives/types"
	"github.com/theapemachine/symm/numeric"
)

func (signal *Signal) publish(trade market.TradeUpdate, reading pumpReading) error {
	if reading.observation == 0 {
		return errnie.Error(errLiftUnobserved)
	}

	category := signal.categories[signal.classifier.Label(reading.code)]

	measurement := types.Measurement{
		At:         trade.Timestamp,
		Symbol:     trade.Symbol,
		Source:     types.SourcePumpDump,
		Category:   category,
		Last:       trade.Price,
		Strength:   reading.observation,
		Confidence: reading.confidence,
	}

	if err := types.AssignCategorySNR(
		&measurement, signal.floor, reading.standout,
	); err != nil {
		return errnie.Error(err, "pumpdump: snr %s", trade.Symbol)
	}

	signal.calibrator.Observe(reading.observation, signal.classifier)

	telemetry := signal.calibrator.Snapshot(signal.classifier)
	telemetry.Observation = reading.observation

	if err := measurement.Send(signal.pool); err != nil {
		return errnie.Error(err, "pumpdump: send %s", trade.Symbol)
	}

	if ui := signal.broadcasts["ui"]; ui != nil {
		ui.Send(&qpool.QValue[any]{
			Value: numeric.GaugePayload(
				measurement.Source.String(),
				measurement.Symbol,
				measurement.Category,
				measurement,
				telemetry,
			),
		})
	}

	if err := signal.rawDump.Write(rawRecord{
		TimestampUnixNano: trade.Timestamp.UnixNano(),
		Symbol:            trade.Symbol,
		Price:             trade.Price,
		Qty:               trade.Qty,
		Side:              trade.Side,
		Anchor:            reading.anchor,
		GrossVolume:       reading.grossVolume,
		SignedVolume:      reading.signedVolume,
		RVOL:              reading.rvol,
		Precursor:         reading.precursor,
		Skew:              reading.skew,
		Lift:              reading.lift,
		Observation:       reading.observation,
	}); err != nil {
		errnie.Error(err, "pumpdump: raw dump %s", trade.Symbol)
	}

	return nil
}
