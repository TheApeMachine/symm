package fluid

import (
	"sync"
	"time"

	"github.com/theapemachine/errnie"
)

/*
Registry owns fluid state by symbol so ticker, book, and trade ingestion
converge on one market model.
*/
type Registry struct {
	symbols sync.Map
}

func NewSyncRegistry() *Registry {
	return &Registry{}
}

/*
Close completes the Registry lifecycle contract. Symbol state owns no external
resources, so there is currently nothing to release here.
*/
func (registry *Registry) Close() {
}

/*
SetInstrumentTickSize records exchange tick size for a symbol so its grid uses
authoritative instrument metadata.
*/
func (registry *Registry) SetInstrumentTickSize(symbol string, priceIncrement float64) {
	if registry == nil || symbol == "" || priceIncrement <= 0 {
		return
	}

	state := registry.loadSymbol(symbol)

	if state == nil {
		return
	}

	state.setInstrumentTickSize(priceIncrement)
}

/*
loadSymbol returns or creates registered symbol state so ingestion paths share
one fluid model per market.
*/
func (registry *Registry) loadSymbol(symbol string) *FluidSymbol {
	if raw, ok := registry.symbols.Load(symbol); ok {
		return raw.(*FluidSymbol)
	}

	state, err := NewFluidSymbol(symbol)

	if errnie.Error(err) != nil {
		return nil
	}

	raw, _ := registry.symbols.LoadOrStore(symbol, state)

	return raw.(*FluidSymbol)
}

/*
RangeRows visits each current symbol row for caller-controlled traversal.
eventAt is currently unused because every row retains its own observation time.
*/
func (registry *Registry) RangeRows(eventAt time.Time, visit func(map[string]any) bool) {
	registry.symbols.Range(func(_, value any) bool {
		state, ok := value.(*FluidSymbol)

		if !ok {
			return true
		}

		row := state.Row()

		if row == nil {
			return true
		}

		return visit(row)
	})
}
