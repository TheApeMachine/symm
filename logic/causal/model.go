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
LaggedState is the temporal state a transition is evaluated against: it must
be able to answer "what was coordinate X at or before At - lag", the same
as-of semantics the Relation and causal fits were aligned with.
*/
type LaggedState interface {
	ValueAt(coordinate relation.Coordinate, lag time.Duration) (float64, bool)
}

/*
TransitionModel is the fitted causal/predictive transition for one market
variable:

	Y(t) = Intercept + SelfCoefficient * Y(t - SelfLag)
	       + sum(ParentCoefficients[i] * Parent_i(t - ParentLag_i)) + noise

It is fitted only on real observational history, and its runtime evaluation
honors the same lags: the target's own value is read as-of at SelfLag and
each parent's value as-of at its measured lag, so simulation reproduces the
fitted temporal structure instead of substituting contemporaneous values.
The transition never sees future observations, and simulated MCTS states
never enter its fit.
*/
type TransitionModel struct {
	Target  VariableID
	SelfLag time.Duration
	Parents []AllowedParent
	// ExcludedParents are schema-authorized parent directions with no
	// currently defined Relation (and therefore no measured lag); they are
	// recorded for provenance, not silently activated with a fallback lag.
	ExcludedParents []AllowedParent

	ObservationCount   int
	AlignedCount       int
	ParameterCount     int
	Rank               int
	Reason             string

	Intercept          float64
	SelfCoefficient    float64
	ParentCoefficients []float64
	ResidualVariance   float64
	EffectiveSupport   float64
	Maturity           float64
	FitAt              time.Time
	Status             IdentificationStatus
}

/*
Step returns the expected next value of the target and the residual noise
scale, evaluated as-of on the supplied temporal state. The target's own
value is read at At (lag zero relative to the state) and each parent at
At - (ParentLag - SelfLag), which is the exact lag structure the model was
fitted with; a parent whose required cutoff lies in the future makes the
step undefined. When the transition is not identified, or a required value
is missing, it returns defined=false.
*/
func (model *TransitionModel) Step(state LaggedState) (float64, float64, bool) {
	if model == nil || model.Status != IdentificationIdentified {
		return 0, 0, false
	}

	selfValue, found := state.ValueAt(model.Target.Coordinate, 0)

	if !found {
		return 0, 0, false
	}

	expected := model.Intercept + model.SelfCoefficient*selfValue

	for index, parent := range model.Parents {
		// The fitted model predicts Y(t) from Parent(t - ParentLag). The
		// next value is Y(At + SelfLag), so the parent's required cutoff is
		// At + SelfLag - ParentLag = At - (ParentLag - SelfLag).
		asOfLag := parent.Lag - model.SelfLag

		if asOfLag < 0 {
			// The parent's required value lies in the future of the state.
			return 0, 0, false
		}

		parentValue, parentFound := state.ValueAt(parent.Parent.Coordinate, asOfLag)

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
			Reason: "target variable not declared in schema",
		}
	}

	transition := &TransitionModel{
		Target:  target,
		SelfLag: specification.SelfLag,
		Status:  IdentificationInsufficientSupport,
	}

	if specification.SelfLag <= 0 {
		transition.Status = IdentificationUndefined
		transition.Reason = "self lag must be positive"
		return transition
	}

	targetView, targetFound := model.store.ViewRing(target.Coordinate)

	if !targetFound || targetView.Len() == 0 {
		if targetFound {
			targetView.Close()
		}

		transition.Status = IdentificationUndefined
		transition.Reason = "required target history absent"
		return transition
	}

	defer targetView.Close()

	transition.ObservationCount = targetView.Len()

	// The schema authorizes the possibility of each parent direction; a
	// parent becomes active only when the Influence Graph holds a defined
	// Relation for it, and the Relation's measured lag is then used. An
	// authorized direction without a defined Relation is a candidate-but-
	// unavailable relationship, not an active fitted parent: undefined
	// evidence must not silently become a structural lag assumption. The
	// target's own history (self-lag) is part of the declared transition
	// model and needs no Relation. Excluded parents are recorded for
	// provenance.
	parents := make([]AllowedParent, 0, len(specification.Parents))
	excluded := make([]AllowedParent, 0)

	for _, parent := range specification.Parents {
		if model.influenceGraph == nil {
			excluded = append(excluded, parent)
			continue
		}

		edge := model.influenceGraph.Relation(parent.Parent.Coordinate, target.Coordinate)

		if edge == nil || edge.Result == nil || !edge.Result.Defined() || edge.Result.Lag <= 0 {
			excluded = append(excluded, parent)
			continue
		}

		parents = append(parents, AllowedParent{
			Parent:    parent.Parent,
			Lag:       edge.Result.Lag,
			LagSource: "influence:" + edge.Result.EstimatorVersion,
		})
	}

	transition.Parents = parents
	transition.ExcludedParents = excluded

	series := make([]relation.SeriesView, 0, 1+len(parents))
	series = append(series, relation.SeriesView{
		History: targetView,
		Lag:     specification.SelfLag,
	})

	for _, parent := range parents {
		parentView, parentFound := model.store.ViewRing(parent.Parent.Coordinate)

		if !parentFound || parentView.Len() == 0 {
			if parentFound {
				parentView.Close()
			}

			transition.Status = IdentificationInsufficientSupport
			transition.Reason = "required active parent history absent"
			return transition
		}

		defer parentView.Close()

		series = append(series, relation.SeriesView{
			History: parentView,
			Lag:     parent.Lag,
		})
	}

	aligned := relation.AlignViews(targetView, series)
	transition.AlignedCount = len(aligned)

	if len(aligned) == 0 {
		transition.Status = IdentificationInsufficientSupport
		transition.Reason = "no temporally aligned observations"
		return transition
	}

	parameterCount := 2 + len(parents)
	transition.ParameterCount = parameterCount

	if len(aligned) <= parameterCount {
		transition.Status = IdentificationInsufficientSupport
		transition.Reason = "aligned row count too small for parameter count"
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
	transition.Rank = fit.Rank

	if !fit.Defined {
		transition.Status = IdentificationInsufficientRank
		transition.Reason = "design matrix is not full rank"
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
	transition.Reason = ""

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
earlier rows, and only then incorporated. The normal-equation moments are
maintained incrementally across steps, so the evaluation costs O(n·p²) rather
than refitting the full design at every row. Steps whose fit is not
identifiable contribute no residual.
*/
func prequentialTransitionVariance(aligned []relation.AlignedRow, parameterCount int) float64 {
	accumulator := statistic.NewRegressionAccumulator(parameterCount)
	residuals := make([]float64, 0, len(aligned))

	for _, row := range aligned {
		fit := accumulator.Fit()

		if fit.Defined {
			prediction, _ := fit.Predict(transitionPredictors(row, parameterCount))
			residual := row.Target.Raw - prediction
			residuals = append(residuals, residual*residual)
		}

		accumulator.Add(transitionPredictors(row, parameterCount), row.Target.Raw)
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
transitionPredictors builds one transition design row: intercept, the target's
own lagged value, then each parent's lagged value.
*/
func transitionPredictors(row relation.AlignedRow, parameterCount int) []float64 {
	predictors := make([]float64, 0, parameterCount)
	predictors = append(predictors, 1, row.Predictors[0].Raw)

	for predictorIndex := 1; predictorIndex < len(row.Predictors); predictorIndex++ {
		predictors = append(predictors, row.Predictors[predictorIndex].Raw)
	}

	return predictors
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
		coordinate := marketVariable.Variable.Coordinate
		latest := 0.0
		found := false

		model.store.RangeHistory(coordinate, func(observation relation.Observation) bool {
			if observation.At.After(at) {
				return true
			}

			latest = observation.Raw
			found = true
			return true
		})

		if found {
			state[coordinate] = latest
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

	// A market or outcome coordinate is never directly mutated by a strategy
	// action without an explicit market-impact model, so the treatment
	// effect on it is NotIdentifiable regardless of how the schema labels
	// the variable. There is no action-target escape hatch here.
	if request.Target.Role == RoleMarket || request.Target.Role == RoleOutcome {
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
