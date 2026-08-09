package category

import "github.com/theapemachine/symm/types"

/*
Mapper holds the weight each measured metric carries as evidence for or
against a category. A positive weight supports the category, a negative
weight contradicts it, and the magnitude is how much that one observable is
worth relative to the others voting on the same hypothesis.

Categories are deliberately cross-signal: no single signal names a category,
because the same reading means different things depending on what the rest of
the market is doing. Aggressive drive with thinning depth is a breakout;
aggressive drive into loaded depth is absorption.
*/
type Mapper struct {
	metrics map[types.MetricType]map[types.CategoryType]float64

	/*
		trending holds the weights that read a metric's direction rather than
		its level. Anticipating a move requires knowing what is developing,
		not only what is already true: compression that is still tightening
		is a coil, while compression sitting flat is a quiet market.
	*/
	trending map[types.MetricType]map[types.CategoryType]float64
}

/*
NewMapper returns a Mapper with the metric-to-category weights registered.
*/
func NewMapper() *Mapper {
	mapper := &Mapper{
		metrics:  make(map[types.MetricType]map[types.CategoryType]float64),
		trending: make(map[types.MetricType]map[types.CategoryType]float64),
	}

	for _, metric := range mapper.known() {
		mapper.Update(metric)
	}

	mapper.trends()

	return mapper
}

/*
trends registers the metrics whose direction is itself evidence.

A coiled compression is the setup that precedes a vertical ignition: spread
tightening while arrivals build, with the depth that would explain it away as
a spoof absent. Catching it is worth more than catching the ignition, because
by the time price is vertical the fills are already poor.
*/
func (mapper *Mapper) trends() {
	mapper.trending = map[types.MetricType]map[types.CategoryType]float64{
		/*
			Compression rising means the spread is tightening tick over
			tick, which is the coil winding rather than a spread that
			simply happens to be narrow.
		*/
		types.MetricCompression: {
			types.CategoryCoiledCompression: 1.0,
		},

		/*
			Arrivals clustering ahead of any price move is pressure
			building under a quiet tape.
		*/
		types.MetricConditionalIntensity: {
			types.CategoryCoiledCompression: 1.0,
		},
		types.MetricRVOL: {
			types.CategoryCoiledCompression: 1.0,
		},

		/*
			Depth stacking while nothing trades is the signature of quotes
			posted to be seen rather than filled, so a coil that rests on
			it is not a coil at all.
		*/
		types.MetricLoadedScore: {
			types.CategoryCoiledCompression: -1.0,
			types.CategorySpoofTrap:         1.0,
		},
		types.MetricSpoofScore: {
			types.CategoryCoiledCompression: -1.0,
			types.CategorySpoofTrap:         1.0,
		},

		/*
			A move already underway is the ignition itself, not its
			precursor.
		*/
		types.MetricIgnition: {
			types.CategoryCoiledCompression: -1.0,
		},
	}
}

/*
known lists the metrics that currently carry category affinity. Metrics that
report magnitude only, such as raw counts and strengths, are intentionally
absent because they say how much happened rather than what happened.
*/
func (mapper *Mapper) known() []types.MetricType {
	return []types.MetricType{
		types.MetricEventCount,
		types.MetricCompression,
		types.MetricIgnition,
		types.MetricRVOL,
		types.MetricPrecursor,
		types.MetricTrend,
		types.MetricExhaustion,
		types.MetricConditionalIntensity,
		types.MetricDrive,
		types.MetricAbsorption,
		types.MetricSurgeScore,
		types.MetricThinScore,
		types.MetricLoadedScore,
	}
}

/*
Update registers the category weights for one metric.
*/
func (mapper *Mapper) Update(metricType types.MetricType) {
	switch metricType {
	case types.MetricEventCount:
		mapper.metrics[metricType] = map[types.CategoryType]float64{
			types.CategoryDivergentMove:   1.0,
			types.CategoryAggressiveDrive: -1.0,
		}
	case types.MetricCompression:
		mapper.metrics[metricType] = map[types.CategoryType]float64{
			types.CategoryEndogenousAlpha:   1.0,
			types.CategoryCoiledCompression: 1.0,
			types.CategoryHardSupport:       -1.0,
		}

	/*
		A vertical ignition is a pump caught in the act: price gapping on
		volume far above its own baseline, with order arrivals clustering
		and aggressive buyers driving. No single one of those is a pump,
		which is why the category is assembled from all of them.
	*/
	case types.MetricIgnition:
		mapper.metrics[metricType] = map[types.CategoryType]float64{
			types.CategoryVerticalIgnition:  1.0,
			types.CategoryStochasticBalance: -1.0,
		}
	case types.MetricRVOL:
		mapper.metrics[metricType] = map[types.CategoryType]float64{
			types.CategoryVerticalIgnition: 1.0,
			types.CategoryFrenzy:           1.0,
			types.CategoryVolumeStarvation: -1.0,
		}
	case types.MetricPrecursor:
		mapper.metrics[metricType] = map[types.CategoryType]float64{
			types.CategoryVerticalIgnition: 1.0,
			types.CategoryEndogenousAlpha:  1.0,
		}
	case types.MetricTrend:
		mapper.metrics[metricType] = map[types.CategoryType]float64{
			types.CategoryAggressiveDrive:   1.0,
			types.CategoryStochasticBalance: -1.0,
		}

	/*
		Exhaustion is the counterweight: the same tape that supports a pump
		continuing is evidence against it when buyers stop being replaced.
	*/
	case types.MetricExhaustion:
		mapper.metrics[metricType] = map[types.CategoryType]float64{
			types.CategoryExhaustion:       1.0,
			types.CategoryVerticalIgnition: -1.0,
			types.CategoryFrenzy:           -1.0,
		}
	case types.MetricConditionalIntensity:
		mapper.metrics[metricType] = map[types.CategoryType]float64{
			types.CategoryFrenzy:           1.0,
			types.CategoryVerticalIgnition: 1.0,
			types.CategoryLaminar:          -1.0,
		}
	case types.MetricDrive:
		mapper.metrics[metricType] = map[types.CategoryType]float64{
			types.CategoryAggressiveDrive:   1.0,
			types.CategoryVerticalIgnition:  1.0,
			types.CategoryCoiledCompression: 1.0,
			types.CategoryHiddenAbsorption:  -1.0,
		}
	case types.MetricAbsorption:
		mapper.metrics[metricType] = map[types.CategoryType]float64{
			types.CategoryHiddenAbsorption: 1.0,
			types.CategoryAggressiveDrive:  -1.0,
		}
	case types.MetricSurgeScore:
		mapper.metrics[metricType] = map[types.CategoryType]float64{
			types.CategoryRiskOnSurge:      1.0,
			types.CategoryVerticalIgnition: 1.0,
		}
	case types.MetricThinScore:
		mapper.metrics[metricType] = map[types.CategoryType]float64{
			types.CategoryBookThinning:    1.0,
			types.CategoryLiquidityVacuum: 1.0,
			types.CategoryRobustLiquidity: -1.0,
		}
	case types.MetricLoadedScore:
		mapper.metrics[metricType] = map[types.CategoryType]float64{
			types.CategoryLoadedImbalance: 1.0,
			types.CategoryDenseNeutrality: -1.0,
		}
	}
}

/*
Weights returns the category weights registered for one metric, and whether
that metric carries any category affinity at all.
*/
func (mapper *Mapper) Weights(
	metricType types.MetricType,
) (map[types.CategoryType]float64, bool) {
	weights, ok := mapper.metrics[metricType]

	return weights, ok && len(weights) > 0
}

/*
Trending returns the categories one metric's direction speaks to, and whether
that metric carries any directional affinity at all.
*/
func (mapper *Mapper) Trending(
	metricType types.MetricType,
) (map[types.CategoryType]float64, bool) {
	weights, ok := mapper.trending[metricType]

	return weights, ok && len(weights) > 0
}

/*
Categories returns every category any registered metric can speak to, which
is the set a symbol's evidence is scored against.
*/
func (mapper *Mapper) Categories() []types.CategoryType {
	seen := make(map[types.CategoryType]struct{})
	categories := make([]types.CategoryType, 0)

	for _, weights := range mapper.metrics {
		for category := range weights {
			if _, ok := seen[category]; ok {
				continue
			}

			seen[category] = struct{}{}
			categories = append(categories, category)
		}
	}

	return categories
}

/*
Speaks reports whether a metric carries an opinion about one category, which
is how a category knows which evidence is missing rather than merely absent.
*/
func (mapper *Mapper) Speaks(
	metricType types.MetricType, category types.CategoryType,
) bool {
	weights, ok := mapper.metrics[metricType]

	if !ok {
		return false
	}

	weight, ok := weights[category]

	return ok && weight != 0
}
