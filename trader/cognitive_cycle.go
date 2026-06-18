package trader

import (
	"time"

	"github.com/theapemachine/symm/trader/cognitive"
)

/*
observeCognitiveMeasurements records calibrated story readings for the tick.
*/
func (crypto *Crypto) observeCognitiveMeasurements() {
	if crypto == nil || crypto.memory == nil || crypto.story == nil {
		return
	}

	for _, measurement := range crypto.story.Measurements() {
		crypto.memory.ObserveMeasurement(measurement)
	}
}

/*
sealCognitiveScopes finalizes pending observations for every scope in the tick.
*/
func (crypto *Crypto) sealCognitiveScopes(
	scopes []string,
	eventAt time.Time,
) []*cognitive.Reading {
	if crypto == nil || crypto.memory == nil {
		return nil
	}

	return crypto.memory.SealAllScopes(scopes, eventAt)
}

/*
maybeConsolidateCognitive runs REM replay when the configured interval elapses.
*/
func (crypto *Crypto) maybeConsolidateCognitive(eventAt time.Time) {
	if crypto == nil || crypto.memory == nil {
		return
	}

	crypto.memory.MaybeConsolidate(eventAt)
}
