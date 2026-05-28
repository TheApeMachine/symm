package price

import "github.com/theapemachine/symm/engine"

// RegisterStop arms a stop for one symbol. A later tick at or below stopPrice
// fires exactly one stop_hit exit. Re-registering resets the fired flag.
func (prediction *Prediction) RegisterStop(symbol string, stopPrice float64) {
	if symbol == "" || stopPrice <= 0 {
		return
	}

	prediction.stateMu.Lock()
	prediction.stops[symbol] = stopOrder{price: stopPrice}
	prediction.stateMu.Unlock()
}

// RegisterTrailingStop arms a trailing stop with a hard floor. The trigger is
// the higher of the floor and peak*(1-trailFrac); peak ratchets up with price.
// Used for pump-regime positions (§15.6): there is no time gate, so the
// trailing stop is the sole downside control once a vertical reverses.
func (prediction *Prediction) RegisterTrailingStop(symbol string, hardFloor, trailFrac float64) {
	if symbol == "" || hardFloor <= 0 || trailFrac <= 0 {
		return
	}

	prediction.stateMu.Lock()
	prediction.stops[symbol] = stopOrder{price: hardFloor, trail: true, trailFrac: trailFrac}
	prediction.stateMu.Unlock()
}

// ClearStop disarms the stop for one symbol. Called when the position is
// closed for any reason so a reused symbol does not inherit a stale stop.
func (prediction *Prediction) ClearStop(symbol string) {
	prediction.stateMu.Lock()
	delete(prediction.stops, symbol)
	prediction.stateMu.Unlock()
}

// checkStopLocked returns a stop_hit exit when price breaches an armed,
// unfired stop. A trailing stop ratchets its peak up with price and fires at
// the higher of the hard floor and peak*(1-trailFrac); a fixed stop fires at
// its single level. Must be called with stateMu held; the caller emits after
// releasing the lock.
func (prediction *Prediction) checkStopLocked(symbol string, price float64) (engine.Exit, bool) {
	if symbol == "" || price <= 0 {
		return engine.Exit{}, false
	}

	stop, ok := prediction.stops[symbol]

	if !ok || stop.fired {
		return engine.Exit{}, false
	}

	trigger := stop.price

	if stop.trail {
		if price > stop.peak {
			stop.peak = price
			prediction.stops[symbol] = stop
		}

		if trail := stop.peak * (1 - stop.trailFrac); trail > trigger {
			trigger = trail
		}
	}

	if trigger <= 0 || price > trigger {
		return engine.Exit{}, false
	}

	stop.fired = true
	prediction.stops[symbol] = stop

	return engine.Exit{
		Symbol:     symbol,
		Urgency:    1,
		Reason:     engine.ExitReasonStopHit,
		LimitPrice: trigger,
	}, true
}
