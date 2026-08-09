package category

import (
	"math"
	"strings"

	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

/*
minimumEvidence is how many independent metrics must speak to a category
before it is claimed. One observable agreeing with itself is not a market
regime, it is a reading.
*/
const minimumEvidence = 2

/*
Solver derives categories from the measurements every signal contributed this
tick. A category is a hypothesis about what the market is doing, and each
metric that carries affinity is typed evidence for or against it.
*/
type Solver struct {
	mapper   *Mapper
	api      *websocket.API
	recorder *audit.Recorder
	ui       chan []byte
}

/*
NewSolver creates a new Solver for the category logic.
*/
func NewSolver(
	api *websocket.API,
	ui chan []byte,
	recorder *audit.Recorder,
) *Solver {
	return &Solver{
		mapper:   NewMapper(),
		api:      api,
		recorder: recorder,
		ui:       ui,
	}
}

/*
evidence accumulates one symbol's support for and opposition to a single
category, along with the provenance needed to explain the verdict.
*/
type evidence struct {
	support    float64
	opposition float64
	supporting []string
	opposing   []string
	maturity   float64
	samples    int

	// distinct counts the separate observables that spoke, as opposed to
	// how many rows carried them. Ten readings of one metric is one piece
	// of evidence repeated, not ten independent confirmations.
	distinct map[string]struct{}
}

/*
Update scores every category the mapper knows about against the measurements
this tick carried, and records those that cleared their evidence threshold.
Categories are the substrate the graph and the cognition tree are built from,
so they are derived before either runs.
*/
func (solver *Solver) Update(thesis *types.Thesis) error {
	// Categories are read off this tick's measurements, so there is nothing to
	// classify until every signal has stamped. Skipping leaves the stamp
	// unraised and the tick comes back once the evidence is there.
	if !thesis.SignalsMeasured() {
		return nil
	}

	measurementsBySymbol := make(map[string][]*types.Measurement)

	thesis.Measurements.Range(func(_, value any) bool {
		measurements, ok := value.([]*types.Measurement)

		if !ok {
			return true
		}

		for _, measurement := range measurements {
			if measurement == nil || measurement.Symbol == "" {
				continue
			}

			measurementsBySymbol[measurement.Symbol] = append(
				measurementsBySymbol[measurement.Symbol],
				measurement,
			)
		}

		return true
	})

	for symbol, measurements := range measurementsBySymbol {

		found := solver.classify(symbol, measurements)

		if len(found) == 0 {
			// A symbol the evidence no longer supports must not keep the
			// verdict it carried on an earlier tick.
			thesis.Categories.Delete(symbol)
			continue
		}

		thesis.Categories.Store(symbol, found)
	}

	thesis.Stamp(types.SourceCategories)

	return nil
}

/*
Close releases the solver. Categories are derived per tick and hold no
resources of their own.
*/
func (solver *Solver) Close() error {
	return nil
}

/*
classify weighs one symbol's measurements against every candidate category
and returns those the evidence actually supports.
*/
func (solver *Solver) classify(
	symbol string,
	measurements []*types.Measurement,
) []types.Category {
	tally := make(map[types.CategoryType]*evidence)
	readings := make(map[string][]float64)
	maturity := make(map[string]float64)

	for _, measurement := range measurements {
		for key, sample := range measurement.Metrics {
			if sample.Normalized == nil {
				continue
			}

			reading := *sample.Normalized

			if math.IsNaN(reading) || math.IsInf(reading, 0) {
				continue
			}

			readings[key] = append(readings[key], reading)
			maturity[key] = measurement.Maturity
		}
	}

	for key, samples := range readings {
		metric, _ := types.ParseMetricKey(key)
		level := samples[len(samples)-1]

		/*
			A metric speaks twice: its level says what is true now, and its
			direction says where the market is heading. The two are scored
			independently, because a metric can carry one without the other.
		*/
		if weights, ok := solver.mapper.Weights(metric); ok {
			solver.contribute(key, math.Abs(level), maturity[key], weights, tally)
		}

		if len(samples) < 2 {
			continue
		}

		weights, ok := solver.mapper.Trending(metric)

		if !ok {
			continue
		}

		first := samples[0]
		last := samples[len(samples)-1]
		slope := (last - first) / float64(len(samples)-1)
		solver.contribute(key+trendSuffix, slope, maturity[key], weights, tally)
	}

	return solver.verdicts(symbol, tally)
}

/*
trendSuffix marks evidence drawn from a metric's direction, so a verdict can
show that compression tightening is what carried it rather than the level.
*/
const trendSuffix = "↑"

/*
contribute weighs one reading against every category it speaks to. The
reading's sign decides direction: a rising metric supports the categories it
is positively weighted for, while a falling one supports their opposites.
*/
func (solver *Solver) contribute(
	key string,
	reading float64,
	maturity float64,
	weights map[types.CategoryType]float64,
	tally map[types.CategoryType]*evidence,
) {
	if reading == 0 || math.IsNaN(reading) || math.IsInf(reading, 0) {
		return
	}

	for category, weight := range weights {
		found, ok := tally[category]

		if !ok {
			found = &evidence{distinct: map[string]struct{}{}}
			tally[category] = found
		}

		/*
			Each observable contributes once, at the strength it reads now.
			Repeating a metric across the tick's rows would let a chatty
			signal outvote the rest of the market.
		*/
		contribution := math.Abs(reading) * math.Abs(weight)
		found.samples++
		found.maturity += maturity
		found.distinct[key] = struct{}{}

		if (reading > 0) == (weight > 0) {
			found.support += contribution
			found.supporting = append(found.supporting, key)

			continue
		}

		found.opposition += contribution
		found.opposing = append(found.opposing, key)
	}
}

/*
verdicts turns accumulated evidence into the categories that hold. A category
is claimed when enough independent metrics spoke to it and support outweighs
opposition; the balance of the two becomes its confidence.
*/
func (solver *Solver) verdicts(
	symbol string,
	tally map[types.CategoryType]*evidence,
) []types.Category {
	categories := make([]types.Category, 0, len(tally))

	for category, found := range tally {
		total := found.support + found.opposition

		if len(found.distinct) < minimumEvidence || total == 0 {
			continue
		}

		confidence := found.support / total

		if confidence <= 0.5 {
			continue
		}

		categories = append(categories, types.Category{
			Symbol:     symbol,
			Type:       category,
			Confidence: confidence,
			Strength:   found.support - found.opposition,
			Maturity:   found.maturity / float64(found.samples),

			/*
				Surprisal is the information carried by the contested share
				of the evidence. A category the observables agree on
				unanimously tells us nothing we did not already see; one
				held against real contradiction is the informative case.
			*/
			Surprisal:   -math.Log(math.Max(1.0-confidence, 1e-9)),
			Uncertainty: 1.0 - confidence,
			Supporting:  found.supporting,
			Opposing:    found.opposing,
			Missing:     solver.missing(category, found),
		})
	}

	return categories
}

/*
missing names the metrics that carry an opinion about this category but did
not appear in the tick's evidence, so a verdict can state what it did not
get to see rather than silently assuming it.
*/
func (solver *Solver) missing(
	category types.CategoryType,
	found *evidence,
) []string {
	seen := make(map[types.MetricType]struct{}, len(found.distinct))

	/*
		Evidence is recorded under its wire key, which may name a side, so
		it is resolved back to the metric before asking what never spoke.
	*/
	for key := range found.distinct {
		metric, _ := types.ParseMetricKey(strings.TrimSuffix(key, trendSuffix))
		seen[metric] = struct{}{}
	}

	absent := make([]string, 0)

	for metric := range solver.mapper.metrics {
		if !solver.mapper.Speaks(metric, category) {
			continue
		}

		if _, ok := seen[metric]; ok {
			continue
		}

		absent = append(absent, string(metric))
	}

	for metric := range solver.mapper.trending {
		if _, ok := solver.mapper.trending[metric][category]; !ok {
			continue
		}

		if _, ok := seen[metric]; ok {
			continue
		}

		absent = append(absent, string(metric)+trendSuffix)
	}

	return absent
}
