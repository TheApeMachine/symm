package causal

import (
	"math"
	"sync"
	"time"

	"github.com/theapemachine/symm/logic/graph"
	"github.com/theapemachine/symm/nomagique/relation"
	"github.com/theapemachine/symm/nomagique/statistic"
)

/*
TransitionModel is the fitted causal/predictive transition for one market
variable:

	Y(t + SelfLag) = Intercept + SelfCoefficient * Y(t)
	                 + sum(ParentCoefficients[i] * Parent_i(t)) + noise

It is fitted only on real observational history. The transition never sees
future observations, and simulated MCTS states never enter its fit.
*/
type TransitionModel struct {
	Target   VariableID
	SelfLag  time.Duration
	Parents  []AllowedParent
	// ExcludedParents are schema-authorized parents with no retained
	// observations; they are recorded for provenance, not silently dropped.
	ExcludedParents []AllowedParent

	Intercept         float64
	SelfCoefficient   float64
	ParentCoefficients []float64
	ResidualVariance  float64
	EffectiveSupport  float64
	Maturity          float64
	FitAt             time.Time
	Status            IdentificationStatus
}

/*
Step returns the expected next value of the target and the residual noise
scale, given current coordinate values. When the transition is not
identified, or a required current value is missing, it returns defined=false.
*/
func (model *TransitionModel) Step(current map[relation.Coordinate]float64) (float64, float64, bool) {
	if model == nil || model.Status != IdentificationIdentified {
		return 0, 0, false
	}

	selfValue, found := current[model.Target.Coordinate]

	if !found {
		return 0, 0, false
	}

	expected := model.Intercept + model.SelfCoefficient*selfValue

	for index, parent := range model.Parents {
		parentValue, parentFound := current[parent.Parent.Coordinate]

		if !parentFound {
			return 0, 0, false
		}

		expected += model.ParentCoefficients[index] * parentValue
	}

	noise := 0.0

	if model.ResidualVariance > 0 {
		noise = math.Sqrt(model.ResidualVariance)
	}

	return expected, noise, true
}

/*
CausalModel answers explicit causal questions under one CausalSchema. It
consumes Measurement history, the schema, and Influence Graph evidence; it
never consumes opaque semantic scores as a substitute for market state.
*/
type CausalModel struct {
	mu             sync.RWMutex
	schema         *CausalSchema
	store          *relation.ObservationStore
	influenceGraph *graph.InfluenceGraph
	modelVersion   string
}

/*
NewCausalModel builds the model. The influence graph is optional evidence:
it may nominate candidate parents, but the schema authorizes structure.
*/
func NewCausalModel(
	schema *CausalSchema,
	store *relation.ObservationStore,
	influenceGraph *graph.InfluenceGraph,
	modelVersion string,
) *CausalModel {
	if modelVersion == "" {
		modelVersion = "causal-linear-v1"
	}

	return &CausalModel{
		schema:         schema,
		store:          store,
		influenceGraph: influenceGraph,
		modelVersion:   modelVersion,
	}
}

/*
Schema returns the schema the model operates under.
*/
func (model *CausalModel) Schema() *CausalSchema {
	if model == nil {
		return nil
	}

	return model.schema
}

/*
ModelVersion returns the model version string.
*/
func (model *CausalModel) ModelVersion() string {
	if model == nil {
		return ""
	}

	return model.modelVersion
}

/*
TransitionModel fits the transition model for one market variable using only
observations at or before at. The fit is causal/prequential: the residual
variance is evaluated by predicting each row with a model fitted strictly on
earlier rows.

Feature/parent selection is query-local and schema-authorized: the schema
authorizes which directions are structurally allowed, and the Influence Graph
supplies the measured temporal relationship — the selected lag — for each
authorized parent when a defined Relation exists. A parent whose Relation is
not yet defined falls back to the schema's declared lag; the provenance of
every lag is preserved. The full Measurement history remains available.
*/
func (model *CausalModel) TransitionModel(target VariableID, at time.Time) *TransitionModel {
	if model == nil || model.schema == nil || model.store == nil {
		return &TransitionModel{Target: target, Status: IdentificationUndefined}
	}

	specification, declared := model.schema.MarketVariableFor(target)

	if !declared {
		return &TransitionModel{
			Target: target,
			Status: IdentificationNotIdentifiable,
		}
	}

	transition := &TransitionModel{
		Target:  target,
		SelfLag: specification.SelfLag,
		Status:  IdentificationInsufficientSupport,
	}

	if specification.SelfLag <= 0 {
		return transition
	}

	targetHistory := model.store.History(target.Coordinate)

	if len(targetHistory) == 0 {
		transition.Status = IdentificationUndefined
		return transition
	}

	// Resolve each schema-authorized parent to its measured lag from the
	// Influence Graph when a defined Relation exists; otherwise keep the
	// schema's declared lag. The schema authorizes the direction; the graph
	// provides the observed temporal relationship.
	//
	// A parent with no retained observations is excluded from this query's
	// parent set (query-local projection): the coordinate remains in history
	// for other queries, but the model cannot use a variable it has never
	// observed. Excluded parents are recorded for provenance.
	parents := make([]AllowedParent, 0, len(specification.Parents))
	excluded := make([]AllowedParent, 0)

	for _, parent := range specification.Parents {
		if len(model.store.History(parent.Parent.Coordinate)) == 0 {
			excluded = append(excluded, parent)
			continue
		}

		lag := parent.Lag
		lagSource := "schema"

		if model.influenceGraph != nil {
			if edge := model.influenceGraph.Relation(parent.Parent.Coordinate, target.Coordinate); edge != nil &&
				edge.Result != nil && edge.Result.Defined() && edge.Result.Lag > 0 {
				lag = edge.Result.Lag
				lagSource = "influence:" + edge.Result.EstimatorVersion
			}
		}

		parents = append(parents, AllowedParent{
			Parent:    parent.Parent,
			Lag:       lag,
			LagSource: lagSource,
		})
	}

	transition.Parents = parents
	transition.ExcludedParents = excluded

	series := make([]relation.LaggedSeries, 0, 1+len(parents))
	series = append(series, relation.LaggedSeries{
		Observations: targetHistory,
		Lag:          specification.SelfLag,
	})

	for _, parent := range parents {
		parentHistory := model.store.History(parent.Parent.Coordinate)

		if len(parentHistory) == 0 {
			transition.Status = IdentificationInsufficientSupport
			return transition
		}

		series = append(series, relation.LaggedSeries{
			Observations: parentHistory,
			Lag:          parent.Lag,
		})
	}

	aligned := relation.AlignLagged(targetHistory, series)

	if len(aligned) == 0 {
		transition.Status = IdentificationInsufficientSupport
		return transition
	}

	parameterCount := 2 + len(parents)

	if len(aligned) <= parameterCount {
		transition.Status = IdentificationInsufficientSupport
		return transition
	}

	design := make([]float64, 0, len(aligned)*parameterCount)
	targets := make([]float64, 0, len(aligned))

	for _, row := range aligned {
		design = append(design, 1, row.Predictors[0].Raw)

		for index := 1; index < len(row.Predictors); index++ {
			design = append(design, row.Predictors[index].Raw)
		}

		targets = append(targets, row.Target.Raw)
	}

	fit := statistic.FitOLS(design, targets, parameterCount)

	if !fit.Defined {
		transition.Status = IdentificationInsufficientRank
		return transition
	}

	transition.Intercept = fit.Coefficients[0]
	transition.SelfCoefficient = fit.Coefficients[1]
	transition.ParentCoefficients = append([]float64(nil), fit.Coefficients[2:]...)
	transition.ResidualVariance = prequentialTransitionVariance(aligned, parameterCount)
	transition.FitAt = at

	weights := make([]float64, len(aligned))

	for index := range weights {
		weights[index] = 1
	}

	effective := statistic.EffectiveSampleSize(weights)
	transition.EffectiveSupport = effective
	transition.Maturity = statistic.KishMaturity(weights)
	transition.Status = IdentificationIdentified

	return transition
}

/*
TransitionModels fits the transition model for every schema market variable
at or before at. It is the full time-sliced system the causal rollout
evolves: each market variable's next value depends on its own history and its
schema-authorized, graph-informed parents.
*/
func (model *CausalModel) TransitionModels(at time.Time) map[relation.Coordinate]*TransitionModel {
	if model == nil || model.schema == nil {
		return nil
	}

	transitions := make(map[relation.Coordinate]*TransitionModel, len(model.schema.MarketVariables))

	for _, marketVariable := range model.schema.MarketVariables {
		transitions[marketVariable.Variable.Coordinate] = model.TransitionModel(marketVariable.Variable, at)
	}

	return transitions
}

/*
prequentialTransitionVariance evaluates the transition model's residual
variance prequentially: each row is predicted by a model fitted strictly on
earlier rows, and only then incorporated. Steps whose fit is not identifiable
contribute no residual.
*/
func prequentialTransitionVariance(aligned []relation.AlignedRow, parameterCount int) float64 {
	residuals := make([]float64, 0, len(aligned))

	for index := range aligned {
		if index <= parameterCount {
			continue
		}

		design := make([]float64, 0, index*parameterCount)
		targets := make([]float64, 0, index)

		for rowIndex := 0; rowIndex < index; rowIndex++ {
			design = append(design, 1, aligned[rowIndex].Predictors[0].Raw)

			for predictorIndex := 1; predictorIndex < len(aligned[rowIndex].Predictors); predictorIndex++ {
				design = append(design, aligned[rowIndex].Predictors[predictorIndex].Raw)
			}

			targets = append(targets, aligned[rowIndex].Target.Raw)
		}

		fit := statistic.FitOLS(design, targets, parameterCount)

		if !fit.Defined {
			continue
		}

		predicted := fit.Coefficients[0] + fit.Coefficients[1]*aligned[index].Predictors[0].Raw

		for predictorIndex := 1; predictorIndex < len(aligned[index].Predictors); predictorIndex++ {
			predicted += fit.Coefficients[1+predictorIndex] * aligned[index].Predictors[predictorIndex].Raw
		}

		residual := aligned[index].Target.Raw - predicted
		residuals = append(residuals, residual*residual)
	}

	if len(residuals) == 0 {
		return 0
	}

	sum := 0.0

	for _, residual := range residuals {
		sum += residual
	}

	return sum / float64(len(residuals))
}

/*
MarketState returns the latest observed value of every schema market
coordinate at or before at. Only real observations appear; simulated states
never enter.
*/
func (model *CausalModel) MarketState(at time.Time) map[relation.Coordinate]float64 {
	if model == nil || model.schema == nil || model.store == nil {
		return nil
	}

	state := make(map[relation.Coordinate]float64)

	for _, marketVariable := range model.schema.MarketVariables {
		history := model.store.History(marketVariable.Variable.Coordinate)

		for index := len(history) - 1; index >= 0; index-- {
			if history[index].At.After(at) {
				continue
			}

			state[marketVariable.Variable.Coordinate] = history[index].Raw
			break
		}
	}

	return state
}

/*
Outcome answers one explicit causal question. A treatment effect on a market
coordinate is NotIdentifiable without an explicit market-impact model; an
effect on a portfolio variable is identified deterministically because the
strategy actually controls it.
*/
func (model *CausalModel) Outcome(request OutcomeRequest) *TreatmentEffect {
	if model == nil || model.schema == nil {
		return &TreatmentEffect{
			Treatment: request.Treatment,
			Target:    request.Target,
			At:        request.At,
			Status:    IdentificationUndefined,
		}
	}

	if request.Target.Role == RoleMarket || request.Target.Role == RoleOutcome {
		if model.schema.IsAction(request.Target) {
			return model.schema.PortfolioOutcome(request, request.Current[request.Target], "action directly controls its declared portfolio variable")
		}

		return model.schema.NotIdentifiableOutcome(
			request,
			"strategy actions do not directly mutate market coordinates without an explicit market-impact model; no defensible adjustment set is declared for this market target",
		)
	}

	if request.Target.Role == RolePortfolio {
		expected, defined := model.portfolioEffect(request)

		if !defined {
			return model.schema.NotIdentifiableOutcome(
				request,
				"portfolio effect requires the action to be feasible under the current portfolio state",
			)
		}

		return model.schema.PortfolioOutcome(
			request,
			expected,
			"action directly mutates the declared portfolio variable; market evolution stays governed by the market transition model",
		)
	}

	return model.schema.NotIdentifiableOutcome(
		request,
		"no identification strategy is declared for this variable role",
	)
}

/*
portfolioEffect returns the deterministic portfolio value an action produces
given the current portfolio state in the request.
*/
func (model *CausalModel) portfolioEffect(request OutcomeRequest) (float64, bool) {
	current, found := request.Current[request.Target]

	if !found {
		return 0, false
	}

	switch request.Treatment {
	case "enter":
		if current != 0 {
			return 0, false
		}

		return 1, true
	case "exit":
		if current == 0 {
			return 0, false
		}

		return 0, true
	case "scale":
		if current == 0 {
			return 0, false
		}

		return current + 1, true
	case "wait":
		return current, true
	default:
		return 0, false
	}
}
