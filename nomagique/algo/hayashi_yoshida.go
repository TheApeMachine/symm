package algo

import (
	"github.com/theapemachine/symm/nomagique"
	nmcorrelation "github.com/theapemachine/symm/nomagique/statistic/correlation"
)

/*
NewHayashiYoshida creates the Hayashi-Yoshida asynchronous covariance estimator.
It is a pure composition of Overlap and Correlation primitives with zero implementation code.
*/
func NewHayashiYoshida() *nomagique.Number {
	return nomagique.NewNumber(
		nmcorrelation.NewOverlap(),
		nmcorrelation.NewCorrelation(),
	)
}
