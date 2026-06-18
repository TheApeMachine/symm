package fluid

import "time"

/*
MarketFacts carries quote context used to enrich dashboard measurements.
*/
type MarketFacts struct {
	Price      float64
	Volume     float64
	Spread     float64
	Elapsed    float64
	Surprise   float64
	ObservedAt time.Time
}

/*
MarketFacts reads the latest quote context for one scope from fluid symbol state.
*/
func (signal *Signal) MarketFacts(scope string) MarketFacts {
	if signal == nil || signal.registry == nil || scope == "" {
		return MarketFacts{}
	}

	state := signal.registry.loadSymbol(scope)

	if state == nil {
		return MarketFacts{}
	}

	price := state.last

	if price <= 0 && state.bid > 0 && state.ask > 0 {
		price = (state.bid + state.ask) / 2
	}

	observedAt := state.lastEventAt

	if observedAt.IsZero() {
		observedAt = time.Now()
	}

	surprise := 0.0

	if state.spreadBPS > 0 && price > 0 {
		surprise = state.spreadBPS / 10000.0
	}

	return MarketFacts{
		Price:      price,
		Volume:     state.volume,
		Spread:     state.spreadBPS,
		Elapsed:    0,
		Surprise:   surprise,
		ObservedAt: observedAt,
	}
}
