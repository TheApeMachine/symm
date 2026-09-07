package strategy

import (
	"context"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/nomagique/learning"
	"maps"
	"slices"
	"strings"
)

/* LearningInspector serves coherent on-demand operator views on the workspace owner. */
type LearningInspector struct {
	*LocalLearning
	*Execution
	*PolicyReview
	Capital  *CapitalLearner
	ctx      context.Context
	requests chan learningRequest
}

/* learningRequest crosses into the workspace owner; it never reads live maps. */
type learningRequest struct {
	symbol string
	reply  chan LearningView
}

/* Snapshot asks the single writer for a coherent copy, only on operator demand. */
func (inspector *LearningInspector) Snapshot(ctx context.Context, symbol string) (LearningView, error) {
	if err := ctx.Err(); err != nil {
		return LearningView{}, err
	}

	request := learningRequest{symbol: symbol, reply: make(chan LearningView, 1)}

	select {
	case inspector.requests <- request:
	case <-ctx.Done():
		return LearningView{}, ctx.Err()
	case <-inspector.ctx.Done():
		return LearningView{}, inspector.ctx.Err()
	}

	select {
	case view := <-request.reply:
		return view, nil
	case <-ctx.Done():
		return LearningView{}, ctx.Err()
	case <-inspector.ctx.Done():
		return LearningView{}, inspector.ctx.Err()
	}
}

/* view runs exclusively on the workspace owner, off the ordinary hot path. */
func (inspector *LearningInspector) view(symbol string) LearningView {
	view := LearningView{At: inspector.now(), Symbol: symbol, Status: "waiting for market observations",
		Steps: inspector.steps, Decisions: inspector.decisions, Resolved: inspector.resolved,
		GridVersion: inspector.Grid.Version, Columns: len(inspector.Grid.Columns), InitialCapital: inspector.initial.String(),
		Dispatched: inspector.dispatched, Rejected: inspector.rejected,
		Influence: inspector.attribution.report(inspector.Grid.Columns)}

	view.Warmup = inspector.Warmed
	view.Capital = CapitalView{Choice: inspector.Capital.LastChoice, Prior: inspector.Capital.LastReading.Selected, Evidence: inspector.Capital.LastReading, Decisions: inspector.Capital.Decisions,
		WarmupUnverified: inspector.Capital.History.Unverified,
		Outcomes:         append([]hindsight.CandidateResult(nil), inspector.Capital.Candidates.recent...)}
	demand := zero
	for _, candidate := range inspector.Capital.Candidates.current {
		view.Capital.Candidates = append(view.Capital.Candidates, CandidateView{CandidateRecord: candidate.Record, State: candidate.State, Current: candidate.Current(view.At), Age: view.At.Sub(candidate.Record.At)})

		if candidate.Current(view.At) && !candidate.selected {
			demand = demand.Add(candidate.cost)
		}
	}
	view.Capital.Demand = demand.String()
	slices.SortFunc(view.Capital.Candidates, func(left, right CandidateView) int { return strings.Compare(left.Symbol, right.Symbol) })
	for teacher, output := range map[*AccountTeacher]*AccountLearningView{inspector.Capital.Actual: &view.Capital.Actual, inspector.Capital.Exploration: &view.Capital.Exploration} {
		*output = AccountLearningView{State: teacher.State, Outcome: teacher.Outcome, Target: teacher.Target, Resolved: teacher.Resolved, Aborted: teacher.Aborted, Execution: teacher.LastExecution, MFE: teacher.MFE, MAE: teacher.MAE,
			TimeToPositive: teacher.TimeToPositive, TimeToBreakeven: teacher.TimeToBreakeven, Holding: teacher.Holding, Trajectory: append([]EquityMark(nil), teacher.Trajectory...)}
		output.State.Positions = maps.Clone(teacher.State.Positions)

		if teacher.pending != nil {
			output.Pending = teacher.pending.ID
			output.Horizon, output.HorizonSource = teacher.pending.horizon, teacher.pending.horizonSource
			output.PendingState = "observing WAIT"

			if teacher.pending.receipt != nil {
				output.PendingState = "awaiting execution"
			}

			if teacher.pending.execution != nil {
				output.PendingState = teacher.pending.execution.State
			}
		}
	}

	if inspector.Skill != nil {
		view.Skill = inspector.Skill.Reading()
	}

	view.AuthorizedMode = inspector.Mode().String()

	if inspector.Realization != nil {
		view.RealizationAllowed = inspector.Realization.AllowsTrading()
		view.RealizationReason = inspector.Realization.Reason()
	} else {
		view.RealizationAllowed = true
	}

	if inspector.Balance != nil {
		view.Execution, view.HasExecution = inspector.Stats(), true
	}

	view.Forward = inspector.forward
	view.Forward.Recent = append([]MissedOpportunity(nil), inspector.forward.Recent...)

	if inspector.lastRejection != nil {
		view.Rejection = inspector.lastRejection.Error()
	}

	for key, market := range inspector.markets {
		summary := LearningSummary{Symbol: key, Status: market.status}
		for _, lane := range market.lanes {
			summary.Decisions += lane.issued
		}
		view.Universe = append(view.Universe, summary)
	}

	slices.SortFunc(view.Universe, func(left, right LearningSummary) int {
		if left.Decisions > right.Decisions {
			return -1
		}

		if left.Decisions < right.Decisions {
			return 1
		}

		if left.Symbol < right.Symbol {
			return -1
		}

		if left.Symbol > right.Symbol {
			return 1
		}
		return 0
	})

	if symbol == "" && len(view.Universe) > 0 {
		symbol = view.Universe[0].Symbol
	}
	market := inspector.markets[symbol]

	if market == nil {
		return view
	}
	view.Symbol, view.Status = symbol, market.status
	view.Regions = append([]learning.Region(nil), market.regions...)
	view.PrecursorDepth = len(market.history)

	for _, pastState := range market.history {
		var pastTokens []LearningToken

		for _, conditionToken := range pastState {
			rawID := int(conditionToken & 0xFFFF)
			token := LearningToken{Token: conditionToken}

			if rawID > 0 && rawID <= len(inspector.Grid.Columns) {
				token.Source, token.Label = inspector.Grid.Columns[rawID-1][0], inspector.Grid.Columns[rawID-1][1]
			}

			pastTokens = append(pastTokens, token)
		}

		view.PrecursorHistory = append(view.PrecursorHistory, pastTokens)
	}

	for _, region := range market.regions {
		token := LearningToken{Token: region.ID, Strength: region.Strength,
			Authority: region.Authority, Members: region.Members}

		if index := int(region.ID) - 1; index >= 0 && index < len(inspector.Grid.Columns) {
			token.Source, token.Label = inspector.Grid.Columns[index][0], inspector.Grid.Columns[index][1]
		}

		view.Impulse = append(view.Impulse, token)
	}

	accountState := "flat"

	if len(market.lanes) > 0 {
		policyLane := &market.lanes[len(market.lanes)-1]
		accountState = policyLane.wallet.state()
	}

	if len(market.context) > 0 {
		selected, _, err := inspector.Knowledge.Select(symbol, accountState, market.context, market.actions, false)

		for _, candidate := range market.actions {
			reading := inspector.Knowledge.Reading(symbol, accountState, market.context, candidate)
			view.Candidates = append(view.Candidates, LearningCandidate{
				Kind:      string(candidate.Kind),
				Power:     candidate.Power,
				Reduce:    candidate.Reduce,
				Selected:  err == nil && candidate == selected,
				Prior:     reading.Selected,
				Knowledge: reading,
				Economic:  reading.Economic,
			})
		}
	}

	for index, lane := range market.lanes {
		mode := "virtual"

		if lane.paper {
			mode = "policy"
		}
		view.Lanes = append(view.Lanes, LearningWallet{Lane: index, Mode: mode,
			Cash: lane.wallet.cash.String(), Quantity: lane.wallet.quantity.String(), Fees: lane.wallet.fees.String(),
			Equity: lane.equity, Profit: lane.outcome.TotalReward, Rate: lane.outcome.Rate, Complete: lane.complete,
			At: lane.outcome.Through.At, Action: lane.action, Pending: lane.pending != 0,
			Issued: lane.issued, Fills: lane.fills, Resolved: lane.resolved, Unresolved: len(lane.trace), Prior: lane.lastPrior,
			Episodes: lane.episodes, Realized: lane.realized, Spent: lane.spent, Exhausted: lane.exhausted})
	}

	for row, key := range inspector.Grid.Rows {
		if key != symbol {
			continue
		}
		activity, quality, err := inspector.Grid.Activity(symbol)

		if err != nil {
			view.Status = err.Error()
			return view
		}
		for column, identity := range inspector.Grid.Columns {
			point := inspector.Grid.Coordinates[column]
			view.Points = append(view.Points, LearningPoint{ID: uint64(column + 1), Source: identity[0], Label: identity[1],
				X: point[0], Y: point[1], Value: inspector.Grid.Values[row][column], Energy: activity[column] * activity[column],
				Authority: quality[column], Present: inspector.Grid.Present[row][column]})
		}
	}

	return view
}
