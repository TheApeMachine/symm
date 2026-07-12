package types

import (
	"math"

	"github.com/theapemachine/errnie"
)

/*
Validate checks that a typed measurement retains enough identity, time, scale,
and validity information for logic to compare it without consulting its signal
implementation. Legacy measurements remain outside this contract during the
explicit migration period.
*/
func (measurement Measurement) Validate() error {
	if !measurement.Typed() {
		return errnie.Err(
			errnie.Validation,
			"measurement: typed metric identity required",
			nil,
		)
	}

	if err := measurement.validateIdentity(); err != nil {
		return err
	}

	if err := measurement.validateCompatibility(); err != nil {
		return err
	}

	if err := measurement.validateMetric(); err != nil {
		return err
	}

	if err := measurement.validateTime(); err != nil {
		return err
	}

	return measurement.validateEstimate()
}

func (measurement Measurement) validateIdentity() error {
	if measurement.Source == "" || measurement.Subject == "" ||
		measurement.Stream == "" || measurement.Symbol == "" ||
		measurement.Unit == "" {
		return errnie.Err(
			errnie.Validation,
			"measurement: source, subject, stream, symbol, and unit required",
			nil,
		)
	}

	return nil
}

func (measurement Measurement) validateCompatibility() error {
	if measurement.Status != "" || measurement.Elapsed != 0 ||
		measurement.EntryBaseline != 0 || measurement.ExitBaseline != 0 ||
		len(measurement.Categories) != 0 || measurement.Metrics != nil {
		return errnie.Err(
			errnie.Validation,
			"measurement: typed evidence cannot contain legacy interpretation fields",
			nil,
		)
	}

	return nil
}

func (measurement Measurement) validateTime() error {
	if measurement.At.IsZero() || measurement.ObservedFrom.IsZero() {
		return errnie.Err(
			errnie.Validation,
			"measurement: observation interval required",
			nil,
		)
	}

	if measurement.ObservedFrom.After(measurement.At) || measurement.Horizon < 0 {
		return errnie.Err(
			errnie.Validation,
			"measurement: invalid observation interval",
			nil,
		)
	}

	if measurement.Horizon != measurement.At.Sub(measurement.ObservedFrom) {
		return errnie.Err(
			errnie.Validation,
			"measurement: horizon does not match observation interval",
			nil,
		)
	}

	if measurement.Scale.Kind == "" || measurement.Scale.From.IsZero() ||
		measurement.Scale.Through.IsZero() ||
		measurement.Scale.From.After(measurement.Scale.Through) ||
		measurement.Scale.Through.After(measurement.At) {
		return errnie.Err(
			errnie.Validation,
			"measurement: valid scale interval required",
			nil,
		)
	}

	return nil
}

func (measurement Measurement) validateEstimate() error {
	if !finiteMeasurementValue(measurement.Raw) ||
		measurement.Maturity < 0 || measurement.Maturity > 1 {
		return errnie.Err(
			errnie.Validation,
			"measurement: finite raw value and bounded maturity required",
			nil,
		)
	}

	if measurement.Normalized.Available &&
		!finiteMeasurementValue(measurement.Normalized.Value) {
		return errnie.Err(
			errnie.Validation,
			"measurement: normalized value must be finite when available",
			nil,
		)
	}

	if !measurement.Normalized.Available && measurement.Normalized.Value != 0 {
		return errnie.Err(
			errnie.Validation,
			"measurement: unavailable normalized value must be empty",
			nil,
		)
	}

	if err := measurement.Validity.Validate(); err != nil {
		return err
	}

	return measurement.Uncertainty.Validate()
}

/*
Validate keeps readiness limitations explicit, which prevents a successful fit
from silently becoming a forecast-ready estimate.
*/
func (validity MeasurementValidity) Validate() error {
	stateValid := false

	switch validity.State {
	case ValidityValid, ValidityProvisional, ValidityInvalid:
		stateValid = true
	}

	readinessValid := false

	switch validity.Readiness {
	case ReadinessObservation, ReadinessIntensity, ReadinessModel, ReadinessForecast:
		readinessValid = true
	}

	if !stateValid || !readinessValid {
		return errnie.Err(
			errnie.Validation,
			"measurement: validity state and readiness required",
			nil,
		)
	}

	if validity.State != ValidityValid && validity.Reason == "" {
		return errnie.Err(
			errnie.Validation,
			"measurement: provisional or invalid estimate requires a reason",
			nil,
		)
	}

	return nil
}

/*
Validate accepts absent uncertainty as an honest limitation and checks every
field when an estimator actually reports an interval.
*/
func (uncertainty MeasurementUncertainty) Validate() error {
	if !uncertainty.Available {
		if uncertainty.Lower != 0 || uncertainty.Upper != 0 ||
			uncertainty.Confidence != 0 || uncertainty.Method != "" {
			return errnie.Err(
				errnie.Validation,
				"measurement: unavailable uncertainty must be empty",
				nil,
			)
		}

		return nil
	}

	if !finiteMeasurementValue(uncertainty.Lower) ||
		!finiteMeasurementValue(uncertainty.Upper) ||
		uncertainty.Lower > uncertainty.Upper ||
		uncertainty.Confidence <= 0 || uncertainty.Confidence > 1 ||
		uncertainty.Method == "" {
		return errnie.Err(
			errnie.Validation,
			"measurement: valid uncertainty interval required",
			nil,
		)
	}

	return nil
}

func finiteMeasurementValue(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
