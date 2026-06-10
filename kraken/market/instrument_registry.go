package market

import "sync"

/*
InstrumentRegistry stores the latest exchange instrument rules per symbol.
*/
type InstrumentRegistry struct {
	constraints sync.Map
}

func NewInstrumentRegistry() *InstrumentRegistry {
	return &InstrumentRegistry{}
}

func (registry *InstrumentRegistry) Observe(update *InstrumentUpdate) {
	if registry == nil || update == nil {
		return
	}

	for _, pair := range update.Pairs {
		if pair.Symbol == "" || pair.Status != "online" {
			continue
		}

		registry.constraints.Store(pair.Symbol, ConstraintsFromPair(pair))
	}
}

func (registry *InstrumentRegistry) Constraints(symbol string) (InstrumentConstraints, bool) {
	if registry == nil {
		return InstrumentConstraints{}, false
	}

	raw, ok := registry.constraints.Load(symbol)

	if !ok {
		return InstrumentConstraints{}, false
	}

	constraints, typed := raw.(InstrumentConstraints)

	return constraints, typed
}
