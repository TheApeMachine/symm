package logic

import "sync"

/*
Holdings is a symbol-quantity snapshot passed into tree evaluation by Story.
*/
type Holdings struct {
	quantities sync.Map
}

func NewHoldings() *Holdings {
	return &Holdings{}
}

func (holdings *Holdings) SetQuantity(symbol string, quantity float64) {
	if quantity <= 0 {
		holdings.quantities.Delete(symbol)
		return
	}

	holdings.quantities.Store(symbol, quantity)
}

func (holdings *Holdings) Quantity(symbol string) float64 {
	if holdings == nil {
		return 0
	}

	raw, ok := holdings.quantities.Load(symbol)

	if !ok {
		return 0
	}

	quantity, ok := raw.(float64)

	if !ok {
		return 0
	}

	return quantity
}

func (holdings *Holdings) IsHolding(symbol string) bool {
	return holdings.Quantity(symbol) > 0
}

func (holdings *Holdings) OpenCount() int {
	if holdings == nil {
		return 0
	}

	count := 0

	holdings.quantities.Range(func(_, value any) bool {
		quantity, ok := value.(float64)

		if ok && quantity > 0 {
			count++
		}

		return true
	})

	return count
}

/*
HoldingSubject gates branches on whether Story reports inventory for the
evaluated symbol.
*/
type HoldingSubject struct {
	Held bool `yaml:"held"`
}
