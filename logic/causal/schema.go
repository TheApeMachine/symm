/*
Package causal converts Measurements and Relation structure into causal and
counterfactual estimates under an explicit versioned CausalSchema.

It distinguishes association, predictive temporal Influence, assumed
structural parents, and identified causal effects. Predictive Influence is
not automatically causality; a causal estimate is produced only under an
explicit identification strategy, and every non-identifiable question returns
an explicit NotIdentifiable state rather than a fallback.

The normative contract is logic/causal/README.md. Where this code and the
README disagree, the README wins.
*/
package causal

import (
	"fmt"
	"time"

	"github.com/theapemachine/symm/nomagique/relation"
)

/*
VariableRole is the explicit role of one variable in the schema. Metric names
never determine roles; the schema does.
*/
type VariableRole uint8

const (
	// RoleMarket is an actual Measurement coordinate (market variable).
	RoleMarket VariableRole = iota
	// RoleTreatment is a strategy action variable.
	RoleTreatment
	// RoleOutcome is an explicit outcome variable.
	RoleOutcome
	// RolePortfolio is a portfolio/account variable the strategy actually
	// controls (position, cash, order state).
	RolePortfolio
	// RoleContext is an exogenous/context variable.
	RoleContext
)

func (role VariableRole) String() string {
	switch role {
	case RoleMarket:
		return "market"
	case RoleTreatment:
		return "treatment"
	case RoleOutcome:
		return "outcome"
	case RolePortfolio:
		return "portfolio"
	case RoleContext:
		return "context"
	default:
		return "unknown"
	}
}

/*
VariableID is the typed identity of one schema variable: a Measurement
coordinate plus its explicit role. Variable identity is never inferred from
metric names.
*/
type VariableID struct {
	Coordinate relation.Coordinate
	Role       VariableRole
}

func (id VariableID) String() string {
	return fmt.Sprintf("%s[%s]", id.Coordinate.ID(), id.Role.String())
}

/*
AllowedParent is one schema-authorized structural parent of a market
variable, with its lag. The schema authorizes the direction; the measured
lag, when an Influence relation exists, comes from the Influence Graph.
*/
type AllowedParent struct {
	Parent VariableID
	Lag    time.Duration
	// LagSource records where the lag came from: "schema" (the fallback
	// declared here) or "influence:<estimator-version>" (measured by the
	// Relation layer).
	LagSource string
}

/*
MarketVariable is the transition specification for one market coordinate:
its own history lag and its schema-authorized lagged parents.
*/
type MarketVariable struct {
	Variable VariableID
	SelfLag  time.Duration
	Parents  []AllowedParent
}

/*
ActionDefinition names one strategic intervention and the portfolio variable
it mutates. Without an explicit market-impact model an action must not mutate
market coordinates.
*/
type ActionDefinition struct {
	Name     string
	Variable VariableID
}

/*
VariablePair is one explicit direction between two variables.
*/
type VariablePair struct {
	From VariableID
	To   VariableID
}

/*
CausalSchema is the versioned wiring contract of one causal model. It defines
variable identities, roles, time semantics, allowed and forbidden
directions, treatment/action variables, outcomes, portfolio variables,
context variables, and the model epoch. It is the wiring mechanism; metric
names are not.
*/
type CausalSchema struct {
	Version uint64
	Epoch   uint64
	Name    string
	Symbol  string

	MarketVariables  []MarketVariable
	Actions          []ActionDefinition
	Outcomes         []VariableID
	PortfolioVariables []VariableID
	ContextVariables []VariableID
	Forbidden        []VariablePair
}

/*
NewCausalSchema builds an empty schema for one epoch.
*/
func NewCausalSchema(name string, symbol string, epoch uint64) *CausalSchema {
	return &CausalSchema{
		Version: 1,
		Epoch:   epoch,
		Name:    name,
		Symbol:  symbol,
	}
}

/*
AddMarketVariable appends one market variable transition specification.
*/
func (schema *CausalSchema) AddMarketVariable(variable MarketVariable) *CausalSchema {
	if schema == nil {
		return nil
	}

	schema.MarketVariables = append(schema.MarketVariables, variable)
	return schema
}

/*
AddAction appends one strategic intervention definition.
*/
func (schema *CausalSchema) AddAction(action ActionDefinition) *CausalSchema {
	if schema == nil {
		return nil
	}

	schema.Actions = append(schema.Actions, action)
	return schema
}

/*
AddOutcome appends one outcome variable.
*/
func (schema *CausalSchema) AddOutcome(variable VariableID) *CausalSchema {
	if schema == nil {
		return nil
	}

	schema.Outcomes = append(schema.Outcomes, variable)
	return schema
}

/*
AddPortfolioVariable appends one portfolio variable.
*/
func (schema *CausalSchema) AddPortfolioVariable(variable VariableID) *CausalSchema {
	if schema == nil {
		return nil
	}

	schema.PortfolioVariables = append(schema.PortfolioVariables, variable)
	return schema
}

/*
AddContextVariable appends one context variable.
*/
func (schema *CausalSchema) AddContextVariable(variable VariableID) *CausalSchema {
	if schema == nil {
		return nil
	}

	schema.ContextVariables = append(schema.ContextVariables, variable)
	return schema
}

/*
ForbidDirection marks one structural direction forbidden (for example
future-to-past edges).
*/
func (schema *CausalSchema) ForbidDirection(from VariableID, to VariableID) *CausalSchema {
	if schema == nil {
		return nil
	}

	schema.Forbidden = append(schema.Forbidden, VariablePair{From: from, To: to})
	return schema
}

/*
MarketVariableFor returns the transition specification for a variable, if it
is a declared market variable.
*/
func (schema *CausalSchema) MarketVariableFor(variable VariableID) (MarketVariable, bool) {
	if schema == nil {
		return MarketVariable{}, false
	}

	for _, marketVariable := range schema.MarketVariables {
		if marketVariable.Variable == variable {
			return marketVariable, true
		}
	}

	return MarketVariable{}, false
}

/*
ForSymbol returns a deep copy of the schema bound to one market symbol: every
coordinate symbol is rewritten and the schema symbol is set. It is how a
symbol-agnostic schema template becomes the per-symbol wiring contract
without duplicating the definition.
*/
func (schema *CausalSchema) ForSymbol(symbol string) *CausalSchema {
	if schema == nil {
		return nil
	}

	cloned := &CausalSchema{
		Version:           schema.Version,
		Epoch:             schema.Epoch,
		Name:              schema.Name,
		Symbol:            symbol,
		MarketVariables:   make([]MarketVariable, 0, len(schema.MarketVariables)),
		Actions:           make([]ActionDefinition, 0, len(schema.Actions)),
		Outcomes:          make([]VariableID, 0, len(schema.Outcomes)),
		PortfolioVariables: make([]VariableID, 0, len(schema.PortfolioVariables)),
		ContextVariables:  make([]VariableID, 0, len(schema.ContextVariables)),
		Forbidden:         make([]VariablePair, 0, len(schema.Forbidden)),
	}

	rewrite := func(variable VariableID) VariableID {
		variable.Coordinate.Symbol = symbol
		variable.Coordinate.Epoch = schema.Epoch
		return variable
	}

	for _, marketVariable := range schema.MarketVariables {
		parents := make([]AllowedParent, 0, len(marketVariable.Parents))

		for _, parent := range marketVariable.Parents {
			parents = append(parents, AllowedParent{Parent: rewrite(parent.Parent), Lag: parent.Lag})
		}

		cloned.MarketVariables = append(cloned.MarketVariables, MarketVariable{
			Variable: rewrite(marketVariable.Variable),
			SelfLag:  marketVariable.SelfLag,
			Parents:  parents,
		})
	}

	for _, action := range schema.Actions {
		cloned.Actions = append(cloned.Actions, ActionDefinition{
			Name:     action.Name,
			Variable: rewrite(action.Variable),
		})
	}

	for _, outcome := range schema.Outcomes {
		cloned.Outcomes = append(cloned.Outcomes, rewrite(outcome))
	}

	for _, portfolio := range schema.PortfolioVariables {
		cloned.PortfolioVariables = append(cloned.PortfolioVariables, rewrite(portfolio))
	}

	for _, context := range schema.ContextVariables {
		cloned.ContextVariables = append(cloned.ContextVariables, rewrite(context))
	}

	for _, pair := range schema.Forbidden {
		cloned.Forbidden = append(cloned.Forbidden, VariablePair{
			From: rewrite(pair.From),
			To:   rewrite(pair.To),
		})
	}

	return cloned
}

/*
IsAction reports whether the variable is declared as an action variable.
*/
func (schema *CausalSchema) IsAction(variable VariableID) bool {
	if schema == nil {
		return false
	}

	for _, action := range schema.Actions {
		if action.Variable == variable {
			return true
		}
	}

	return false
}

/*
DirectionForbidden reports whether the From→To direction is explicitly
forbidden.
*/
func (schema *CausalSchema) DirectionForbidden(from VariableID, to VariableID) bool {
	if schema == nil {
		return false
	}

	for _, pair := range schema.Forbidden {
		if pair.From == from && pair.To == to {
			return true
		}
	}

	return false
}

/*
ColumnIndex maps matrix columns back to schema variables. The mapping
Column ↔ VariableID ↔ Coordinate is stable within one schema version and
reversible.
*/
type ColumnIndex struct {
	// Columns is the stable variable order; column i maps to Columns[i].
	Columns []VariableID
}

/*
ColumnOf returns the column index of a variable, or -1.
*/
func (index *ColumnIndex) ColumnOf(variable VariableID) int {
	if index == nil {
		return -1
	}

	for column, entry := range index.Columns {
		if entry == variable {
			return column
		}
	}

	return -1
}

/*
VariableOf returns the variable at a column index.
*/
func (index *ColumnIndex) VariableOf(column int) (VariableID, bool) {
	if index == nil || column < 0 || column >= len(index.Columns) {
		return VariableID{}, false
	}

	return index.Columns[column], true
}

/*
MarketMatrix is one reversible materialization of the schema: its columns are
generated from the explicit schema, and every column maps back to a
VariableID and a Coordinate. Values are the latest observed values at the
materialization time; a column without data carries an explicit missing
marker, never a fabricated zero.
*/
type MarketMatrix struct {
	Schema *CausalSchema
	Index  ColumnIndex
	Values []float64
	// Missing marks columns without an observed value at At.
	Missing []bool
	At      time.Time
}

/*
Materialize builds the market matrix for the schema at time at, using the
latest retained observation of each market coordinate. Unobserved coordinates
are marked missing, not zero.
*/
func (schema *CausalSchema) Materialize(store *relation.ObservationStore, at time.Time) (*MarketMatrix, error) {
	if schema == nil || store == nil {
		return nil, fmt.Errorf("causal: schema and store are required")
	}

	matrix := &MarketMatrix{
		Schema:  schema,
		Index:   ColumnIndex{Columns: make([]VariableID, 0, len(schema.MarketVariables))},
		Values:  make([]float64, 0, len(schema.MarketVariables)),
		Missing: make([]bool, 0, len(schema.MarketVariables)),
		At:      at,
	}

	for _, marketVariable := range schema.MarketVariables {
		matrix.Index.Columns = append(matrix.Index.Columns, marketVariable.Variable)

		observation, found := store.Latest(marketVariable.Variable.Coordinate)

		if !found {
			matrix.Values = append(matrix.Values, 0)
			matrix.Missing = append(matrix.Missing, true)
			continue
		}

		matrix.Values = append(matrix.Values, observation.Raw)
		matrix.Missing = append(matrix.Missing, false)
	}

	return matrix, nil
}
