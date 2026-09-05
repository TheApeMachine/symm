package strategy

import (
	"context"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/nomagique/learning"
	"maps"
	"math/big"
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
		Dispatched: inspector.dispatched, Rejected: inspector.rejected, HorizonEpochs: horizonEpochs,
		Influence: inspector.attribution.report(inspector.Grid.Columns)}

	view.Warmup = inspector.Warmed
	view.Capital = CapitalView{Choice: inspector.Capital.LastChoice, Prior: inspector.Capital.LastPrior, Decisions: inspector.Capital.Decisions,
		Outcomes: append([]hindsight.CandidateResult(nil), inspector.Capital.Candidates.recent...)}
	demand := new(big.Rat)
	for _, candidate := range inspector.Capital.Candidates.current {
		view.Capital.Candidates = append(view.Capital.Candidates, CandidateView{CandidateRecord: candidate.Record, State: candidate.State, Current: candidate.Current(view.At), Age: view.At.Sub(candidate.Record.At)})

		if candidate.Current(view.At) && !candidate.selected {
			demand.Add(demand, candidate.cost)
		}
	}
	view.Capital.Demand = demand.RatString()
	slices.SortFunc(view.Capital.Candidates, func(left, right CandidateView) int { return strings.Compare(left.Symbol, right.Symbol) })
	for teacher, output := range map[*AccountTeacher]*AccountLearningView{inspector.Capital.Actual: &view.Capital.Actual, inspector.Capital.Exploration: &view.Capital.Exploration} {
		*output = AccountLearningView{State: teacher.State, Outcome: teacher.Outcome, Target: teacher.Target, Resolved: teacher.Resolved, MFE: teacher.MFE, MAE: teacher.MAE,
			TimeToPositive: teacher.TimeToPositive, TimeToBreakeven: teacher.TimeToBreakeven, Holding: teacher.Holding, Trajectory: append([]EquityMark(nil), teacher.Trajectory...)}
		output.State.Positions = maps.Clone(teacher.State.Positions)

		if teacher.pending != nil {
			output.Pending = teacher.pending.ID
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

	if reporter, ok := inspector.Desk.(ExecutionReporter); ok && reporter != nil {
		view.Execution, view.HasExecution = reporter.Execution(), true
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
	view.Horizon, view.EpochMean, view.Epochs = market.horizon(), market.epochMean, market.epochs

	for _, region := range market.regions {
		token := LearningToken{Token: region.ID, Strength: region.Strength,
			Authority: region.Authority, Members: region.Members}

		if index := int(region.ID) - 1; index >= 0 && index < len(inspector.Grid.Columns) {
			token.Source, token.Label = inspector.Grid.Columns[index][0], inspector.Grid.Columns[index][1]
		}

		view.Impulse = append(view.Impulse, token)
	}

	// The last context and feasible set belong to the policy lane, which runs
	// last. Recall never creates evidence, so inspecting it cannot train.
	if len(market.context) > 0 {
		selected, _, err := inspector.Knowledge.Select(symbol, market.context, market.actions, false)

		for _, candidate := range market.actions {
			view.Candidates = append(view.Candidates, LearningCandidate{
				Kind: string(candidate.Kind), Power: candidate.Power, Reduce: candidate.Reduce,
				Selected:  err == nil && candidate == selected,
				Prior:     inspector.Knowledge.Reading(symbol, market.context, candidate).Selected,
				Knowledge: inspector.Knowledge.Reading(symbol, market.context, candidate),
			})
		}
	}

	for index, lane := range market.lanes {
		mode := "virtual"

		if lane.paper {
			mode = "policy"
		}
		view.Lanes = append(view.Lanes, LearningWallet{Lane: index, Mode: mode,
			Cash: lane.wallet.cash.FloatString(lane.wallet.scale), Quantity: lane.wallet.quantity.FloatString(lane.wallet.pair.QtyPrecision), Fees: lane.wallet.fees.FloatString(lane.wallet.scale),
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
