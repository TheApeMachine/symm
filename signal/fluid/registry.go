package fluid

import (
	"fmt"
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
func (registry *Registry) SetInstrumentTickSize(
	symbol string,
	priceIncrement float64,
) error {
	if registry == nil || symbol == "" || priceIncrement <= 0 {
		return fmt.Errorf("fluid: positive instrument tick size and symbol required")
	}

	state, err := registry.loadSymbol(symbol)

	if err != nil {
		return err
	}

	state.setInstrumentTickSize(priceIncrement)

	return nil
}

/*
loadSymbol returns or creates registered symbol state so ingestion paths share
one fluid model per market.
*/
func (registry *Registry) loadSymbol(symbol string) (*FluidSymbol, error) {
	if raw, ok := registry.symbols.Load(symbol); ok {
		return raw.(*FluidSymbol), nil
	}

	state, err := NewFluidSymbol(symbol)

	if err != nil {
		return nil, errnie.Err(
			errnie.UnprocessableContent,
			"fluid: create symbol state for "+symbol,
			err,
		)
	}

	raw, _ := registry.symbols.LoadOrStore(symbol, state)

	return raw.(*FluidSymbol), nil
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
