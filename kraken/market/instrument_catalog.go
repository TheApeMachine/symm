package market

import "sync"

/*
InstrumentCatalog stores the latest instrument channel pair rules keyed by symbol.
Checksum formatting uses price_precision and qty_precision from this catalog.
*/
type InstrumentCatalog struct {
	mu    sync.RWMutex
	pairs map[string]InstrumentPair
}

var sharedInstrumentCatalog = &InstrumentCatalog{
	pairs: make(map[string]InstrumentPair),
}

/*
SharedInstrumentCatalog returns the process-wide instrument rules catalog.
*/
func SharedInstrumentCatalog() *InstrumentCatalog {
	return sharedInstrumentCatalog
}

/*
Apply merges one instrument channel snapshot into the catalog.
*/
func (catalog *InstrumentCatalog) Apply(update InstrumentUpdate) {
	catalog.mu.Lock()
	defer catalog.mu.Unlock()

	for _, pair := range update.Pairs {
		if pair.Symbol == "" {
			continue
		}

		catalog.pairs[pair.Symbol] = pair
	}
}

/*
Pair returns instrument rules for a symbol when the catalog has seen them.
*/
func (catalog *InstrumentCatalog) Pair(symbol string) (InstrumentPair, bool) {
	catalog.mu.RLock()
	defer catalog.mu.RUnlock()

	pair, ok := catalog.pairs[symbol]

	return pair, ok
}
