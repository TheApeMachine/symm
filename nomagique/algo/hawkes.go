package algo

import (
	"github.com/theapemachine/symm/nomagique/data"
	nmhawkes "github.com/theapemachine/symm/nomagique/statistic/hawkes"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
HawkesConfig configures the Hawkes Algo node.
*/
type HawkesConfig struct {
	Clock types.Node
	Mark  types.Node
	Store *store.KeyStore
	Key   func() string
}

/*
Hawkes is the canonical Tier 5 literature Algo for online bivariate Hawkes arrival process estimation.
*/
type Hawkes struct {
	bivariate *nmhawkes.Bivariate
}

/*
NewHawkes constructs a new Hawkes algorithm node.
*/
func NewHawkes(config HawkesConfig) *Hawkes {
	keyFn := config.Key

	if keyFn == nil && config.Store != nil {
		keyFn = config.Store.Key
	}

	return &Hawkes{
		bivariate: nmhawkes.NewBivariateWithKey(config.Clock, config.Mark, config.Store, keyFn),
	}
}

func (hawkes *Hawkes) Step(mark types.Scalar) types.Scalar {
	return hawkes.bivariate.Step(mark)
}

func (hawkes *Hawkes) Measurement() *data.Measurement[float64] {
	return hawkes.bivariate.Measurement()
}

var _ types.Node = (*Hawkes)(nil)
