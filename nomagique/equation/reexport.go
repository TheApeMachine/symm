package equation

import (
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/temporal"
)

/*
Re-exported canonical domain types to preserve vocabulary at the equation boundary.
Per nomagique architecture rules, implementation code belongs to canonical domain
packages while equation contains only composition.
*/
type Price = temporal.Price
type Moments = statistic.Moments
type MomentReading = statistic.MomentReading
type CausalResidualResult = statistic.CausalResidualResult
type LocalRegressionReading = statistic.LocalRegressionReading
type PriorMoments = statistic.PriorMoments
type PriorSummary = statistic.PriorSummary

type Interval = temporal.Interval
type IntervalPair = temporal.IntervalPair
type LogReturn = temporal.LogReturn
