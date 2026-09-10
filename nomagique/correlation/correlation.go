package correlation

import "github.com/theapemachine/symm/nomagique/equation"

/*
Correlation is the energy-normalized covariance owner. The implementation lives
with the other typed equations; this alias keeps the package vocabulary.
*/
type Correlation = equation.Correlation[float64]

func NewCorrelation() *equation.Correlation[float64] {
	return equation.NewCorrelation[float64]()
}
