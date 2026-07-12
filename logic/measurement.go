package logic

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/types"
)

/*
MeasurementAnalyzer is the synchronous measurement entrypoint for logic. It
keeps the legacy evidence adapter visible during migration while every typed
measurement is validated, aligned, and journaled without another observer or
queue between the trading loop and its Thesis.
*/
type MeasurementAnalyzer struct {
	composer *Composer
	thesis   *strategy.Thesis
}

/*
NewMeasurementAnalyzer binds exact-time composition to the existing in-process
Thesis so both manifold and numerical evidence retain one lifecycle carrier.
*/
func NewMeasurementAnalyzer(thesis *strategy.Thesis) *MeasurementAnalyzer {
	return &MeasurementAnalyzer{
		composer: NewComposer(),
		thesis:   thesis,
	}
}

/*
Ingest validates the complete batch before changing the Thesis. Typed values
become chronological logic epochs; unmigrated values retain the previous
latest-by-source behavior until their producers adopt numerical identity.
*/
func (analyzer *MeasurementAnalyzer) Ingest(
	measurements []*types.Measurement,
) error {
	typed, legacy, err := analyzer.partition(measurements)

	if err != nil {
		return err
	}

	epochs, err := analyzer.composer.Compose(typed)

	if err != nil {
		return err
	}

	if err := analyzer.thesis.RecordEpochs(epochs); err != nil {
		return err
	}

	for _, measurement := range legacy {
		analyzer.thesis.AddEvidence(
			measurement.Symbol,
			string(measurement.Source),
			measurement,
		)
	}

	return nil
}

func (analyzer *MeasurementAnalyzer) partition(
	measurements []*types.Measurement,
) (
	typed []*types.Measurement,
	legacy []*types.Measurement,
	err error,
) {
	for _, measurement := range measurements {
		if measurement == nil {
			return nil, nil, errnie.Err(
				errnie.Validation,
				"logic measurement: nil measurement",
				nil,
			)
		}

		if measurement.Symbol == "" || measurement.Source == "" {
			return nil, nil, errnie.Err(
				errnie.Validation,
				"logic measurement: source and symbol required",
				nil,
			)
		}

		if measurement.Typed() {
			typed = append(typed, measurement)

			continue
		}

		legacy = append(legacy, measurement)
	}

	return typed, legacy, nil
}
