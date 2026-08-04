package types

import (
	"math"
	"strconv"
	"strings"
	"sync"
)

/*
PassageOutcome is which boundary an open lot reached first.

These are competing outcomes, not independent events: reaching one ends the
episode, so exactly one of them is true of any lot that finished. A lot that was
closed for some other reason has no outcome at all and is censored rather than
counted as a timeout — recording an early manual exit as "reached neither" would
teach the model that patience is safe using the very cases where patience was
never tested.
*/
type PassageOutcome string

const (
	// OutcomeProfitFirst is a lot that reached its protected profit line before
	// its hard floor.
	OutcomeProfitFirst PassageOutcome = "profit_first"
	// OutcomeLossFirst is a lot that reached its hard floor first.
	OutcomeLossFirst PassageOutcome = "loss_first"
	// OutcomeTimeout is a lot whose forecast horizon expired with neither
	// boundary reached.
	OutcomeTimeout PassageOutcome = "timeout"
)

/*
passageOutcomes is the fixed outcome order every count vector uses.
*/
var passageOutcomes = [3]PassageOutcome{
	OutcomeProfitFirst, OutcomeLossFirst, OutcomeTimeout,
}

/*
passageShrinkage is how much evidence a bucket needs before it outweighs its
parent. At this many observations a bucket's own frequencies and its parent's
carry equal weight, so a bucket seen twice barely moves off the broader
population and one seen hundreds of times is essentially speaking for itself.
*/
const passageShrinkage = 12.0

/*
passageReadySupport is how many finished episodes the model needs, in total,
before anything is allowed to act on what it says.

Below this the estimates are the prior wearing a bucket's name. The gate exists
because this probability governs an exit: a model with no evidence must produce
no opinion rather than a confident-looking one drawn from three trades.
*/
const passageReadySupport = 40.0

/*
passageLocalSupport is how much evidence must stand behind the specific state
being asked about, as opposed to the model as a whole.

A global gate alone is not enough and the difference is not academic. A model
that has learned a great deal about deep drawdowns knows nothing about shallow
ones, and asking it about a shallow one returns the uniform prior — which, with
only a global gate, arrives marked ready and exits the position on a coin flip.
Evidence about one state is not evidence about another.

The threshold matches the shrinkage constant, so a state is actionable exactly
when its own history carries at least as much weight as the population it was
generalised from.
*/
const passageLocalSupport = passageShrinkage

/*
PassageFeatures is the state one open lot is in, as the model sees it.

Everything here is stated relative to the lot's own geometry rather than in
currency: a fifty-cent drawdown means nothing on its own, and means quite a lot
when the hard floor is sixty cents away. That normalisation is what lets one
symbol's history inform another's.
*/
type PassageFeatures struct {
	/*
		Drawdown is how far the mark sits from entry as a share of the risk
		distance. −1 is exactly at the hard floor, 0 is at entry, positive is in
		profit.
	*/
	Drawdown float64
	/*
		Age is elapsed time as a share of the forecast horizon the lot was
		entered on. At 1 the forecast that justified the trade has expired.
	*/
	Age float64
	/*
		Forecast is the current executable expected return as a fraction of
		price — what the model that is still watching this symbol thinks is
		left.
	*/
	Forecast float64
	/*
		Liquidity is the current execution-noise band as a multiple of the one
		the lot was entered under. Above 1 the book has widened since entry, so
		the same distance costs more to cross.
	*/
	Liquidity float64
	/*
		Regime is the structural class the symbol is in. It is a string because
		it comes from the category vocabulary and has no ordering.
	*/
	Regime string
}

/*
PassageEpisode is one finished lot, written out in full so the model that
replaces this one can be fitted offline from the record rather than from
whatever happened to be in memory.

The in-process model learns from these too, but it is not the reason they
exist. Its buckets and shrinkage are a first version; the calibrated
adverse-excursion quantile that should replace RiskMultiples, and the
competing-risk model that should replace the buckets, both need a corpus of
complete episodes with their features and their outcome. Holding thirty-two
geometry changes in memory and losing them on restart is not that corpus.
*/
type PassageEpisode struct {
	PositionID string         `json:"position_id"`
	Symbol     string         `json:"symbol"`
	OpenedTick int64          `json:"opened_tick"`
	ClosedTick int64          `json:"closed_tick"`
	Horizon    float64        `json:"horizon"`
	Outcome    PassageOutcome `json:"outcome"`
	Censored   bool           `json:"censored"`
	ExitReason string         `json:"exit_reason"`
	HardFloor  float64        `json:"hard_floor"`
	ProfitLine float64        `json:"profit_line"`
	Entry      float64        `json:"entry"`
	/*
		MaxAdverse and MaxFavorable are the deepest drawdown and the highest
		excursion the lot reached, in risk distances. They are the raw material
		for the conditional adverse-excursion quantile: the question "how far do
		trades like this go against me before they work" is answered from these
		and nothing else.
	*/
	MaxAdverse   float64           `json:"max_adverse"`
	MaxFavorable float64           `json:"max_favorable"`
	Observations []PassageFeatures `json:"observations"`
}

/*
PassageScenario is the model's answer for one state: the probability of each
competing outcome, and how much evidence stands behind it.
*/
type PassageScenario struct {
	ProfitFirst float64 `json:"profit_first"`
	LossFirst   float64 `json:"loss_first"`
	Timeout     float64 `json:"timeout"`
	/*
		Support is how many finished episodes landed in the most specific bucket
		this estimate used. It is reported separately from the probabilities
		because a 70% drawn from four episodes and a 70% drawn from four hundred
		are not the same claim.
	*/
	Support float64 `json:"support"`
	/*
		Ready is false until the model has seen enough finished episodes overall
		to be worth acting on. A consumer must not exit on a scenario that is
		not ready — the numbers are still the prior.
	*/
	Ready bool `json:"ready"`
}

/*
HoldEV prices continuing to hold against closing now.

	profit × remaining upside − loss × remaining downside − timeout × opportunity

Every term is in the same unit as its input, so a caller working in fractions of
price gets a fraction of price back. A negative result means the position is
worth more closed than held — not that it is certain to lose, only that its
distribution no longer pays for the room it needs.
*/
func (scenario PassageScenario) HoldEV(upside, downside, opportunity float64) float64 {
	value := scenario.ProfitFirst*upside -
		scenario.LossFirst*downside -
		scenario.Timeout*opportunity

	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}

	return value
}

/*
passageCounts is one bucket's tally of finished episodes.
*/
type passageCounts struct {
	counts [3]float64
	total  float64
}

/*
PassageModel estimates which boundary an open lot will reach first, from the
lots that already finished.

It is a hierarchy of empirical buckets rather than a fitted model, because the
question it answers is being asked long before there is enough data to fit
anything. Each bucket shrinks toward its parent — a coarser description of the
same state — so a state seen twice answers almost as the broader population
does, and one seen a thousand times answers for itself. Nothing ever reaches 0%
or 100%, because the top of the hierarchy is a uniform prior that no amount of
evidence can be subtracted from.

The zero value is usable and answers with the prior.
*/
type PassageModel struct {
	mutex   sync.RWMutex
	buckets map[string]*passageCounts
	total   float64
}

/*
NewPassageModel constructs an empty model.
*/
func NewPassageModel() *PassageModel {
	return &PassageModel{buckets: map[string]*passageCounts{}}
}

/*
keys describes one state from coarsest to finest.

Each key is a prefix of the next, so every bucket has exactly one parent and the
hierarchy is a tree. The order is deliberate: regime and drawdown are what most
separates outcomes, and liquidity is the finest split because it changes fastest
and would otherwise fragment the counts.
*/
func (features PassageFeatures) keys() []string {
	regime := features.Regime

	if regime == "" {
		regime = "unclassified"
	}

	drawdown := bucketFloat(features.Drawdown, 0.25)
	age := bucketFloat(features.Age, 0.25)
	forecast := bucketSign(features.Forecast)
	liquidity := bucketFloat(features.Liquidity, 0.5)

	parts := []string{
		"d" + drawdown,
		"r" + regime,
		"a" + age,
		"f" + forecast,
		"l" + liquidity,
	}

	keys := make([]string, 0, len(parts))

	for index := range parts {
		keys = append(keys, strings.Join(parts[:index+1], "|"))
	}

	return keys
}

/*
Observe folds one single-state episode into every bucket that describes it.
Call ObserveEpisode when one episode was observed more than once.
*/
func (model *PassageModel) Observe(features PassageFeatures, outcome PassageOutcome) {
	model.ObserveEpisode([]PassageFeatures{features}, outcome)
}

/*
ObserveEpisode folds one finished episode into every distinct state bucket it
visited.

An episode contributes at most one count to a bucket, however many ticks it
spent there. The outcome belongs to the position, not to each observation of
the position: counting every tick as another result lets one slow loss
manufacture enough nominal support to make the model actionable. Distinct
states are retained so an episode that genuinely moved through the feature
space can still inform each state it reached.
*/
func (model *PassageModel) ObserveEpisode(
	observations []PassageFeatures,
	outcome PassageOutcome,
) {
	if model == nil || len(observations) == 0 {
		return
	}

	index := -1

	for position, candidate := range passageOutcomes {
		if candidate == outcome {
			index = position
			break
		}
	}

	if index < 0 {
		return
	}

	visited := map[string]struct{}{}

	for _, features := range observations {
		for _, key := range features.keys() {
			visited[key] = struct{}{}
		}
	}

	model.mutex.Lock()
	defer model.mutex.Unlock()

	if model.buckets == nil {
		model.buckets = map[string]*passageCounts{}
	}

	for key := range visited {
		bucket, found := model.buckets[key]

		if !found {
			bucket = &passageCounts{}
			model.buckets[key] = bucket
		}

		bucket.counts[index]++
		bucket.total++
	}

	model.total++
}

/*
Total is how many finished episodes the model has folded in.
*/
func (model *PassageModel) Total() float64 {
	if model == nil {
		return 0
	}

	model.mutex.RLock()
	defer model.mutex.RUnlock()

	return model.total
}

/*
Scenario estimates the competing outcomes for one live state.

It walks the hierarchy from the uniform prior downward, and at each level blends
that level's observed frequencies with what the level above already concluded.
A level with no observations passes its parent through untouched, so a state the
model has never seen in detail still gets the best answer its coarser
description supports.
*/
func (model *PassageModel) Scenario(features PassageFeatures) PassageScenario {
	estimate := [3]float64{1.0 / 3, 1.0 / 3, 1.0 / 3}
	support := 0.0

	if model == nil {
		return PassageScenario{
			ProfitFirst: estimate[0],
			LossFirst:   estimate[1],
			Timeout:     estimate[2],
		}
	}

	model.mutex.RLock()

	for _, key := range features.keys() {
		bucket, found := model.buckets[key]

		if !found || bucket.total <= 0 {
			continue
		}

		weight := bucket.total / (bucket.total + passageShrinkage)

		for index := range estimate {
			observed := bucket.counts[index] / bucket.total
			estimate[index] = weight*observed + (1-weight)*estimate[index]
		}

		support = bucket.total
	}

	total := model.total

	model.mutex.RUnlock()

	estimate = floorEstimate(estimate, support)

	return PassageScenario{
		ProfitFirst: estimate[0],
		LossFirst:   estimate[1],
		Timeout:     estimate[2],
		Support:     support,
		Ready:       total >= passageReadySupport && support >= passageLocalSupport,
	}
}

/*
floorEstimate keeps every outcome above the smallest probability the evidence
could actually justify, then renormalises.

The hierarchy shrinks toward its parent at every level, and when the same
episodes populate all of those levels the prior's contribution decays once per
level — five levels of unanimous evidence drive an outcome to roughly one in ten
million, which no number of trades supports. The floor is one in
observations-plus-outcomes: two hundred consistent episodes justify saying "less
than one in two hundred", and nothing justifies saying "never".
*/
func floorEstimate(estimate [3]float64, support float64) [3]float64 {
	floor := 1 / (math.Max(0, support) + float64(len(estimate)))
	sum := 0.0

	for index := range estimate {
		estimate[index] = math.Max(estimate[index], floor)
		sum += estimate[index]
	}

	if sum <= 0 {
		return [3]float64{1.0 / 3, 1.0 / 3, 1.0 / 3}
	}

	for index := range estimate {
		estimate[index] /= sum
	}

	return estimate
}

/*
bucketFloat discretises a continuous feature onto a grid, clamped so a runaway
value cannot mint an unbounded number of buckets.
*/
func bucketFloat(value, width float64) string {
	if math.IsNaN(value) || math.IsInf(value, 0) || width <= 0 {
		return "na"
	}

	bin := int(math.Floor(value / width))

	if bin < -8 {
		bin = -8
	}

	if bin > 8 {
		bin = 8
	}

	return strconv.Itoa(bin)
}

/*
bucketSign reduces a forecast to whether there is anything left in it. The
magnitude is deliberately dropped: expected return is the noisiest input here,
and splitting on its size fragments the counts without separating the outcomes.
*/
func bucketSign(value float64) string {
	switch {
	case math.IsNaN(value) || math.IsInf(value, 0):
		return "na"
	case value > 0:
		return "pos"
	case value < 0:
		return "neg"
	default:
		return "flat"
	}
}
