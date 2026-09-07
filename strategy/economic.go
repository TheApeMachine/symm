package strategy

import (
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
)

/*
EconomicMoments owns reliability-weighted moments for both log wealth growth
and elapsed time, along with their causal clock. It preserves the two facts
separately so the long-run objective is the ratio of sums, never the arithmetic
mean of independently normalized rates.
*/
type EconomicMoments struct {
	Samples       uint64
	Pending       uint64
	LastEpoch     uint64
	GrowthMean    float64
	GrowthMoment  float64
	GrowthWeight  float64
	GrowthSupport float64
	TotalGrowth   float64
	TotalTime     float64
	TimeMean      float64
}

/*
EconomicReading exposes completion, support, dispersion, separate wealth and
time expectations, and the derived supported compounded growth rate.
*/
type EconomicReading struct {
	Depth             int     `json:"depth"`
	ContextLength     int     `json:"contextLength"`
	Pending           uint64  `json:"pending"`
	Samples           uint64  `json:"samples"`
	Defined           bool    `json:"defined"`
	VarianceDefined   bool    `json:"varianceDefined"`
	GrowthMean        float64 `json:"growthMean"`
	GrowthVariance    float64 `json:"growthVariance"`
	TimeMean          float64 `json:"timeMean"`
	TotalGrowth       float64 `json:"totalGrowth"`
	TotalTime         float64 `json:"totalTime"`
	Rate              float64 `json:"rate"`
	Support           float64 `json:"support"`
	Maturity          float64 `json:"maturity"`
	EvidenceAuthority float64 `json:"evidenceAuthority"`
	Authority         float64 `json:"authority"`
	Memory            float64 `json:"memory"`
}

/* SamplingVariance evaluates the canonical specificity-debt equation on wealth growth. */
func (reading EconomicReading) SamplingVariance() float64 {
	variance, err := equation.SamplingVariance(
		float64(reading.Depth), float64(reading.ContextLength), reading.Support, reading.GrowthVariance,
	)

	if err != nil {
		panic(err)
	}

	return variance
}

/* Age discounts retained weight on causal clock advances. */
func (moments *EconomicMoments) Age(epoch uint64, memory float64) {
	if epoch <= moments.LastEpoch {
		return
	}

	if memory > 1 {
		gap := float64(epoch - moments.LastEpoch)
		discount := math.Exp(gap * math.Log(1-1/memory))
		moments.GrowthWeight *= discount
	}

	moments.LastEpoch = epoch
}

/*
Observe records one resolved economic transition with separate wealth growth,
elapsed time, and issue-time observation authority.
*/
func (moments *EconomicMoments) Observe(
	growth float64, elapsed float64, authority float64, memory float64, epoch ...uint64,
) error {
	if authority < 0 || authority > 1 {
		return fmt.Errorf("%w: economic authority must be in [0, 1]", core.ErrDomain)
	}

	if elapsed <= 0 {
		return fmt.Errorf("%w: positive elapsed time required", core.ErrDomain)
	}

	moments.Samples++

	if authority == 0 {
		return nil
	}

	if len(epoch) > 0 {
		moments.Age(epoch[0], memory)
	}

	if len(epoch) == 0 && memory > 1 {
		moments.GrowthWeight *= math.Exp(math.Log(1 - 1/memory))
	}

	weightedGrowth := authority * growth
	weightedTime := authority * elapsed
	moments.TotalGrowth += weightedGrowth
	moments.TotalTime += weightedTime

	if moments.GrowthWeight == 0 {
		moments.GrowthMean = growth
		moments.GrowthWeight = authority
		moments.GrowthSupport = 1
		moments.GrowthMoment = 0
		moments.TimeMean = elapsed
		return nil
	}

	total := moments.GrowthWeight + authority
	retained := moments.GrowthWeight / total
	incoming := authority / total
	growthDifference := growth - moments.GrowthMean
	timeDifference := elapsed - moments.TimeMean

	moments.GrowthSupport = 1 / (retained*retained/moments.GrowthSupport + incoming*incoming)
	moments.GrowthMoment = retained*moments.GrowthMoment + (retained*incoming)*(growthDifference*growthDifference)
	moments.GrowthMean += incoming * growthDifference
	moments.TimeMean += incoming * timeDifference
	moments.GrowthWeight = total
	return nil
}

/* Summary computes the economic reading and supported compounded growth rate. */
func (moments EconomicMoments) Summary(memory float64) EconomicReading {
	reading := EconomicReading{
		Samples:     moments.Samples,
		Pending:     moments.Pending,
		Memory:      memory,
		TotalGrowth: moments.TotalGrowth,
		TotalTime:   moments.TotalTime,
		GrowthMean:  moments.GrowthMean,
		TimeMean:    moments.TimeMean,
	}

	if moments.GrowthWeight <= 0 {
		return reading
	}

	reading.Defined = true
	reading.Support = moments.GrowthSupport
	reading.EvidenceAuthority = moments.GrowthWeight / moments.GrowthSupport

	if moments.TotalTime > 0 {
		reading.Rate = moments.TotalGrowth / moments.TotalTime
	}

	if moments.GrowthSupport <= 1 {
		return reading
	}

	reading.VarianceDefined = true
	reading.GrowthVariance = moments.GrowthMoment * (moments.GrowthSupport / (moments.GrowthSupport - 1))
	reading.Maturity = (moments.GrowthSupport - 1) / moments.GrowthSupport

	power := reading.Rate * reading.Rate
	rateVariance := 0.0

	if moments.TimeMean > 0 {
		rateVariance = reading.GrowthVariance / (moments.TimeMean * moments.TimeMean)
	}

	totalPower := power + rateVariance

	if totalPower > 0 {
		reading.Authority = (reading.Maturity * reading.EvidenceAuthority) * (power / totalPower)
	}

	return reading
}

/*
EconomicModel keeps priors for keyed, ordered precursor contexts and comparable
actions. Keys distinguish symbol and account state (flat vs holding). Context
trie matching is strict sequence matching, preserving within-state order,
frame delimiters, and temporal transitions without permutation collapse.
*/
type EconomicModel struct {
	contexts map[[2]string]*economicNode
	pending  map[uint64]pendingEconomicDecision
	sequence uint64
	memory   float64
}

/* economicNode owns the actions and continuations of one precursor context prefix. */
type economicNode struct {
	epoch    uint64
	children map[uint64]*economicNode
	priors   map[LearningAction]*economicPrior
}

/* economicPrior binds moments to an action. */
type economicPrior struct {
	state   EconomicMoments
	memory  float64
	pending uint64
}

func (prior *economicPrior) reading(epoch uint64) EconomicReading {
	prior.state.Age(epoch, prior.memory)
	summary := prior.state.Summary(prior.memory)
	summary.Pending = prior.pending
	return summary
}

type pendingEconomicDecision struct {
	priors    []scopedEconomicPrior
	authority float64
	depth     int
}

type scopedEconomicPrior struct {
	*economicPrior
	epoch *uint64
}

/* NewEconomicModel constructs a keyed economic prior model. */
func NewEconomicModel(memory ...float64) *EconomicModel {
	model := &EconomicModel{
		contexts: make(map[[2]string]*economicNode),
		pending:  make(map[uint64]pendingEconomicDecision),
	}

	if len(memory) > 0 && memory[0] > 1 {
		model.memory = memory[0]
	}

	return model
}

/*
Issue binds an action to its ordered precursor context and observation authority.
Context matching is strictly ordered. Shorter prefixes correspond to shorter
recent precursor histories; Recall reads the deepest supported reading.
*/
func (model *EconomicModel) Issue(
	key [2]string, context []uint64, action LearningAction, authority float64, related ...[2]string,
) (uint64, error) {
	if authority < 0 || authority > 1 {
		return 0, errnie.Err(errnie.Validation, "economic model: authority must be in [0, 1]", nil)
	}

	priors := make([]scopedEconomicPrior, 0, (len(context)+1)*(len(related)+1))

	for index, scope := range related {
		if scope == key || slices.Contains(related[:index], scope) {
			return 0, errnie.Err(errnie.Validation, "economic model: related scope must differ from primary", nil)
		}
	}

	for _, scope := range related {
		priors = model.bind(scope, context, action, priors)
	}

	priors = model.bind(key, context, action, priors)
	model.sequence++

	for _, prior := range priors {
		prior.pending++
	}

	model.pending[model.sequence] = pendingEconomicDecision{
		priors: priors, authority: authority, depth: len(context),
	}

	return model.sequence, nil
}

func (model *EconomicModel) bind(
	key [2]string, context []uint64, action LearningAction, priors []scopedEconomicPrior,
) []scopedEconomicPrior {
	node := model.contexts[key]

	if node == nil {
		node = &economicNode{}
		model.contexts[key] = node
	}

	epoch := &node.epoch
	priors = append(priors, scopedEconomicPrior{node.prior(action, model.memory), epoch})

	for _, token := range context {
		if node.children == nil {
			node.children = make(map[uint64]*economicNode)
		}

		next := node.children[token]

		if next == nil {
			next = &economicNode{}
			node.children[token] = next
		}

		node = next
		priors = append(priors, scopedEconomicPrior{node.prior(action, model.memory), epoch})
	}

	return priors
}

func (node *economicNode) prior(action LearningAction, memory float64) *economicPrior {
	if node.priors == nil {
		node.priors = make(map[LearningAction]*economicPrior)
	}

	prior := node.priors[action]

	if prior == nil {
		prior = &economicPrior{memory: memory}
		node.priors[action] = prior
	}

	return prior
}

/*
Resolve incorporates an issued action's realized economic transition exactly once:
log wealth growth and positive elapsed time.
*/
func (model *EconomicModel) Resolve(
	identity uint64, growth float64, elapsed time.Duration,
) (EconomicReading, error) {
	elapsedSeconds := elapsed.Seconds()

	if elapsedSeconds <= 0 {
		return EconomicReading{}, errnie.Err(errnie.Validation, "economic model: positive elapsed time required", nil)
	}

	pending, exists := model.pending[identity]

	if !exists {
		return EconomicReading{}, errnie.Err(
			errnie.Validation, "economic model: action was not issued or is already resolved", nil,
		)
	}

	for index, prior := range pending.priors {
		if index == 0 || prior.epoch != pending.priors[index-1].epoch {
			*prior.epoch++
		}

		if err := prior.state.Observe(growth, elapsedSeconds, pending.authority, prior.memory, *prior.epoch); err != nil {
			return EconomicReading{}, errnie.Error(err)
		}

		prior.pending--
	}

	delete(model.pending, identity)

	last := pending.priors[len(pending.priors)-1]
	reading := last.reading(*last.epoch)
	reading.Depth = pending.depth
	reading.ContextLength = pending.depth
	return reading, nil
}

/* Abort releases an unrealized action without creating evidence. */
func (model *EconomicModel) Abort(identity uint64) error {
	pending, exists := model.pending[identity]

	if !exists {
		return errnie.Err(errnie.Validation, "economic model: action was not issued or already finished", nil)
	}

	for _, prior := range pending.priors {
		prior.pending--
	}

	delete(model.pending, identity)
	return nil
}

/*
Recall traverses strictly ordered prefix tokens. Temporal order (A -> B -> C !=
C -> B -> A), within-state order ([A, B] != [B, A]), and state boundary
delimiters remain distinct paths in the context trie.
*/
func (model *EconomicModel) Recall(key [2]string, context []uint64, action LearningAction) EconomicReading {
	node := model.contexts[key]

	if node == nil {
		return EconomicReading{ContextLength: len(context)}
	}

	epoch := node.epoch
	reading := EconomicReading{}

	if prior := node.priors[action]; prior != nil {
		reading = prior.reading(epoch)
	}

	reading.ContextLength = len(context)
	depth := 0

	for depth < len(context) {
		if node.children == nil {
			break
		}

		token := context[depth]
		next := node.children[token]

		if next == nil {
			break
		}

		node = next
		depth++

		prior := node.priors[action]

		if prior == nil {
			continue
		}

		deeper := prior.reading(epoch)

		if !deeper.Defined {
			continue
		}

		if (deeper.VarianceDefined || !reading.VarianceDefined) &&
			deeper.EvidenceAuthority >= reading.EvidenceAuthority {
			deeper.Depth = depth
			deeper.ContextLength = len(context)
			reading = deeper
		}
	}

	return reading
}

/*
Observe directly incorporates historical evidence without an inflight ticket,
preserving prefix evidence for warmup.
*/
func (model *EconomicModel) Observe(
	key [2]string, context []uint64, action LearningAction, growth float64, elapsed float64, authority float64, related ...[2]string,
) error {
	if authority < 0 || authority > 1 {
		return errnie.Err(errnie.Validation, "economic model: authority must be in [0, 1]", nil)
	}

	if elapsed <= 0 {
		return errnie.Err(errnie.Validation, "economic model: positive elapsed time required", nil)
	}

	priors := make([]scopedEconomicPrior, 0, (len(context)+1)*(len(related)+1))

	for index, scope := range related {
		if scope == key || slices.Contains(related[:index], scope) {
			return errnie.Err(errnie.Validation, "economic model: related scope must differ from primary", nil)
		}
	}

	for _, scope := range related {
		priors = model.bind(scope, context, action, priors)
	}

	priors = model.bind(key, context, action, priors)

	for index, prior := range priors {
		if index == 0 || prior.epoch != priors[index-1].epoch {
			*prior.epoch++
		}

		if err := prior.state.Observe(growth, elapsed, authority, prior.memory, *prior.epoch); err != nil {
			return errnie.Error(err)
		}
	}

	return nil
}

/*
Select evaluates feasible actions against their recalled economic readings.
Without exploration, it selects the action maximizing supported compounded
growth rate. With exploration, it balances unobserved actions first, then
samples from the empirical posterior.
*/
func (model *EconomicModel) Select(
	key [2]string, context []uint64, actions []LearningAction, explore bool,
	recall ...func([2]string, []uint64, LearningAction) EconomicReading,
) (LearningAction, EconomicReading, error) {
	var selected LearningAction
	var selectedReading EconomicReading

	if len(actions) == 0 {
		return selected, selectedReading, errnie.Err(errnie.Validation, "economic model: feasible actions required", nil)
	}

	read := model.Recall

	if len(recall) > 0 {
		read = recall[0]
	}

	best := math.Inf(-1)
	unsupported := false
	least := ^uint64(0)
	start := 0

	if explore {
		start = rand.IntN(len(actions))
	}

	for offset := range actions {
		action := actions[(start+offset)%len(actions)]
		reading := read(key, context, action)
		score := reading.Rate * reading.Authority

		if explore && !reading.VarianceDefined {
			issued := reading.Samples + reading.Pending

			if !unsupported || issued < least {
				selected, selectedReading, least = action, reading, issued
			}

			unsupported = true
			continue
		}

		if unsupported {
			continue
		}

		if explore && reading.TimeMean > 0 {
			rateStdErr := math.Sqrt(reading.SamplingVariance()) / reading.TimeMean
			score += rand.NormFloat64() * rateStdErr
		}

		if score > best {
			selected, selectedReading, best = action, reading, score
		}
	}

	return selected, selectedReading, nil
}
