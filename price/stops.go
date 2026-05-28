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

// ClearStop disarms the stop for one symbol. Called when the position is
// closed for any reason so a reused symbol does not inherit a stale stop.
func (prediction *Prediction) ClearStop(symbol string) {
	prediction.stateMu.Lock()
	delete(prediction.stops, symbol)
	prediction.stateMu.Unlock()
}

// checkStopLocked returns a stop_hit exit when price breaches an armed,
// unfired stop. Must be called with stateMu held; the caller emits after
// releasing the lock.
func (prediction *Prediction) checkStopLocked(symbol string, price float64) (engine.Exit, bool) {
	if symbol == "" || price <= 0 {
		return engine.Exit{}, false
	}

	stop, ok := prediction.stops[symbol]

	if !ok || stop.fired || stop.price <= 0 || price > stop.price {
		return engine.Exit{}, false
	}

	stop.fired = true
	prediction.stops[symbol] = stop

	return engine.Exit{
		Symbol:     symbol,
		Urgency:    1,
		Reason:     engine.ExitReasonStopHit,
		LimitPrice: stop.price,
	}, true
}
