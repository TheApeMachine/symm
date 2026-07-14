package manifold

/*
Advance evolves the field once from every valid population mutation accumulated
since the previous advance.
*/
func (slot *Slot) Advance() ProcessResult {
	result := ProcessResult{}

	if !slot.advanceReady {
		return result
	}

	pending := slot.pending
	slot.pending = pendingObservation{}
	slot.advanceReady = false
	result.Observation = pending.metadata
	result.Accounting = slot.population.Accounting()

	if !slot.population.Ready() {
		return slot.failedResult(pending.metadata, slot.population.InvalidReason())
	}

	at := pending.metadata.At

	if !slot.lastEventAt.IsZero() && at.Before(slot.lastEventAt) {
		return slot.invalidate(pending.metadata, TimestampRegress)
	}

	epoch := slot.population.BeginEpoch()
	orders := slot.population.Orders()
	transform, epochReady := slot.coordinates.BeginEpoch(orders, pending.midPrice, at)

	if !epochReady {
		return slot.failedResult(pending.metadata, UnmappedCarriers)
	}

	mapped, mapReady := slot.mapCarriers(orders, pending.midPrice, at, transform)

	if !mapReady {
		return slot.failedResult(pending.metadata, UnmappedCarriers)
	}

	cohorts := slot.cohorts.Build(mapped)
	eventDeltaT := slot.eventDeltaT(at)
	subdivisions := EventSubdivisions(slot.config, eventDeltaT, cohorts)

	if subdivisions <= 0 {
		return slot.failedResult(pending.metadata, StabilityFailed)
	}

	outcome := slot.step(
		cohorts,
		mapped,
		pending.bestBid,
		pending.bestAsk,
		pending.midPrice,
		at,
		eventDeltaT,
		subdivisions,
		transform,
		epoch,
		result.Accounting,
	)

	if outcome.GasReady {
		slot.lastEventAt = at
	}

	result.State = outcome.State
	result.GasReady = outcome.GasReady
	result.StateProduced = true
	result.CohortCount = len(cohorts)
	result.OrderCount = len(mapped)
	result.DepositCount = outcome.DepositCount
	result.Forecast = outcome.Forecast

	return result
}
