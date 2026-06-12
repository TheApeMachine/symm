package logic

import "sync"

/*
HeldPosition is one open inventory line tracked by Story for tree evaluation.
*/
type HeldPosition struct {
	Quantity        float64
	EntryConfidence float64
}

/*
Holdings is a symbol-position snapshot passed into tree evaluation by Story.
*/
type Holdings struct {
	positions sync.Map
}

func NewHoldings() *Holdings {
	return &Holdings{}
}

func (holdings *Holdings) SetPosition(
	symbol string,
	quantity float64,
	entryConfidence float64,
) {
	if quantity <= 0 {
		holdings.positions.Delete(symbol)
		return
	}

	existing, _ := holdings.HeldPosition(symbol)

	if entryConfidence <= 0 {
		entryConfidence = existing.EntryConfidence
	}

	holdings.positions.Store(symbol, HeldPosition{
		Quantity:        quantity,
		EntryConfidence: entryConfidence,
	})
}

func (holdings *Holdings) SetQuantity(symbol string, quantity float64) {
	holdings.SetPosition(symbol, quantity, 0)
}

func (holdings *Holdings) HeldPosition(symbol string) (HeldPosition, bool) {
	if holdings == nil {
		return HeldPosition{}, false
	}

	raw, ok := holdings.positions.Load(symbol)

	if !ok {
		return HeldPosition{}, false
	}

	position, positionOK := raw.(HeldPosition)

	if !positionOK {
		return HeldPosition{}, false
	}

	return position, true
}

func (holdings *Holdings) Quantity(symbol string) float64 {
	position, ok := holdings.HeldPosition(symbol)

	if !ok {
		return 0
	}

	return position.Quantity
}

func (holdings *Holdings) IsHolding(symbol string) bool {
	return holdings.Quantity(symbol) > 0
}

func (holdings *Holdings) OpenCount() int {
	if holdings == nil {
		return 0
	}

	count := 0

	holdings.positions.Range(func(_, value any) bool {
		position, ok := value.(HeldPosition)

		if ok && position.Quantity > 0 {
			count++
		}

		return true
	})

	return count
}

func (holdings *Holdings) StrictlyHigherConfidenceCount(confidence float64) int {
	if holdings == nil {
		return 0
	}

	count := 0

	holdings.positions.Range(func(_, value any) bool {
		position, ok := value.(HeldPosition)

		if !ok || position.Quantity <= 0 {
			return true
		}

		if position.EntryConfidence > confidence {
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
	Held bool `yaml:"held" json:"held"`
}
