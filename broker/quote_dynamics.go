package broker

import (
	"fmt"
	"sync"

	"github.com/theapemachine/symm/market"
)

/*
QuoteDynamicsRegistry tracks per-symbol spread and slippage history.
It derives anomaly ceilings from each symbol's own recent distribution.
*/
type QuoteDynamicsRegistry struct {
	symbols sync.Map
}

type quoteDynamicsState struct {
	spreadBps   []float64
	slippageBps []float64
}

func NewQuoteDynamicsRegistry() *QuoteDynamicsRegistry {
	return &QuoteDynamicsRegistry{}
}

func (registry *QuoteDynamicsRegistry) Record(quote QuoteSnapshot) {
	if registry == nil || quote.Symbol == "" {
		return
	}

	spreadBps, spreadErr := quote.SpreadBps()

	if spreadErr != nil {
		return
	}

	envelope, envelopeErr := market.LoadDynamicsEnvelope()

	if envelopeErr != nil {
		return
	}

	state := registry.stateFor(quote.Symbol)
	state.spreadBps = market.AppendRingSample(state.spreadBps, spreadBps, envelope.WindowCapacity)

	for _, sample := range quote.slippageSamples() {
		state.slippageBps = market.AppendRingSample(state.slippageBps, sample, envelope.WindowCapacity)
	}
}

func (registry *QuoteDynamicsRegistry) SpreadLimitBps(symbol string) (float64, error) {
	if registry == nil {
		return 0, fmt.Errorf("broker risk: quote dynamics registry is required")
	}

	envelope, envelopeErr := market.LoadDynamicsEnvelope()

	if envelopeErr != nil {
		return 0, envelopeErr
	}

	state := registry.loadState(symbol)
	ceiling, ready := market.AnomalyCeiling(state.spreadBps, envelope)

	if !ready {
		return 0, fmt.Errorf("broker risk: spread baseline for %q is not ready", symbol)
	}

	return ceiling, nil
}

func (registry *QuoteDynamicsRegistry) SlippageLimitBps(
	symbol string,
	spreadLimit float64,
) (float64, error) {
	if registry == nil {
		return 0, fmt.Errorf("broker risk: quote dynamics registry is required")
	}

	envelope, envelopeErr := market.LoadDynamicsEnvelope()

	if envelopeErr != nil {
		return 0, envelopeErr
	}

	state := registry.loadState(symbol)
	ceiling, ready := market.AnomalyCeiling(state.slippageBps, envelope)

	if !ready {
		return 0, fmt.Errorf("broker risk: slippage baseline for %q is not ready", symbol)
	}

	if ceiling > spreadLimit {
		return ceiling, nil
	}

	return spreadLimit, nil
}

func (registry *QuoteDynamicsRegistry) stateFor(symbol string) *quoteDynamicsState {
	rawState, _ := registry.symbols.LoadOrStore(symbol, &quoteDynamicsState{})

	state, ok := rawState.(*quoteDynamicsState)

	if !ok || state == nil {
		state = &quoteDynamicsState{}
		registry.symbols.Store(symbol, state)
	}

	return state
}

func (registry *QuoteDynamicsRegistry) loadState(symbol string) *quoteDynamicsState {
	rawState, ok := registry.symbols.Load(symbol)

	if !ok {
		return &quoteDynamicsState{}
	}

	state, ok := rawState.(*quoteDynamicsState)

	if !ok || state == nil {
		return &quoteDynamicsState{}
	}

	return state
}
