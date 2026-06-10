package logic

/*
Holdings tracks desk inventory consulted by playbook holding conditions.
*/
type Holdings struct {
	quantities map[string]float64
}

func NewHoldings() *Holdings {
	return &Holdings{
		quantities: make(map[string]float64),
	}
}

func (holdings *Holdings) SetQuantity(symbol string, quantity float64) {
	if holdings.quantities == nil {
		holdings.quantities = make(map[string]float64)
	}

	if quantity <= 0 {
		delete(holdings.quantities, symbol)
		return
	}

	holdings.quantities[symbol] = quantity
}

func (holdings *Holdings) Quantity(symbol string) float64 {
	if holdings == nil || holdings.quantities == nil {
		return 0
	}

	return holdings.quantities[symbol]
}

func (holdings *Holdings) IsHolding(symbol string) bool {
	return holdings.Quantity(symbol) > 0
}

func (holdings *Holdings) OpenCount() int {
	if holdings == nil || holdings.quantities == nil {
		return 0
	}

	count := 0

	for _, quantity := range holdings.quantities {
		if quantity > 0 {
			count++
		}
	}

	return count
}

/*
HoldingSubject gates branches on whether the desk holds inventory in the
evaluated symbol.
*/
type HoldingSubject struct {
	Held bool `yaml:"held"`
}
