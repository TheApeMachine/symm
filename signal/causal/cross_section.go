package causal

import (
	"sync"

	"github.com/theapemachine/symm/numeric"
)

type crossSection struct {
	changePct sync.Map
}

func (crossSection *crossSection) publishChangePct(symbol string, changePct float64) {
	crossSection.changePct.Store(symbol, changePct)
}

func (crossSection *crossSection) macroMomentum(symbol string) float64 {
	values := make([]float64, 0, 32)

	crossSection.changePct.Range(func(key, value any) bool {
		peer, ok := key.(string)

		if !ok || peer == symbol {
			return true
		}

		change, ok := value.(float64)

		if !ok {
			return true
		}

		values = append(values, change)

		return true
	})

	if len(values) == 0 {
		return 0
	}

	return numeric.Median(values)
}
