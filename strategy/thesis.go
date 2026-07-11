package strategy

import (
	"sync"

	"github.com/theapemachine/errnie"
)

/*
Thesis is the structure used by the trader to functionally reason
over the compressed and enriched raw market data. It drives the
ultimate decision the trader will make regarding how to enter,
and exit a trade. If at any point during the process the Thesis
breaks down, it is instantly disgarded and no more time is wasted
on it. Just because a Thesis is successfully created, does not
mean it will result in a trade. For that to happen, one or more
of the following needs to be true:

  - A slot must be available for it on the broker.Desk, or it needs to outweigh any
    current occupant of the broker.Desk slots to push a rotation.
  - Failing that, a reserve slow needs to be available, and the Thesis must be eligable
    by clearing the "opportunity" barrier.
  - It needs to outweigh any other Thesis created in the same iteration, or place second
    if multiple slots are available.
*/
type Thesis struct {
	graph    *Graph
	evidence *sync.Map
}

func NewThesis() *Thesis {
	return &Thesis{
		evidence: &sync.Map{},
	}
}

/*
AddEvidence records a stage's snapshot on the Thesis under key.
*/
func (thesis *Thesis) AddEvidence(symbol string, key string, snapshot any) {
	thesis.Update(symbol, *NewEvidence(key, symbol, snapshot))
}

/*
Update records an Evidence snapshot on the Thesis.
*/
func (thesis *Thesis) Update(
	symbol string, evidence Evidence,
) {
	loaded, _ := thesis.evidence.LoadOrStore(symbol, []*Evidence{})
	loaded = append(loaded.([]*Evidence), &evidence)
	thesis.evidence.Store(symbol, loaded)
}

/*
Symbols returns all symbols currently tracked in the thesis.
*/
func (thesis *Thesis) Symbols() []string {
	var symbols []string

	thesis.evidence.Range(func(key, _ any) bool {
		symbols = append(symbols, key.(string))
		return true
	})

	return symbols
}

/*
Values returns the values for a given Symbol.
*/
func (thesis *Thesis) Values(symbol string) ([]*Evidence, error) {
	loaded, ok := thesis.evidence.Load(symbol)

	if !ok {
		return nil, errnie.Error(errnie.Err(
			errnie.NotFound, "symbol not found", nil,
		))
	}

	return loaded.([]*Evidence), nil
}

/*
Evidence returns the snapshot for a given symbol and key.
*/
func (thesis *Thesis) Evidence(symbol string, key string) (any, bool) {
	loaded, err := thesis.Values(symbol)
	if err != nil {
		return nil, false
	}

	for _, ev := range loaded {
		if ev.Source == key {
			return ev.Snapshot, true
		}
	}

	return nil, false
}
