package strategy

import (
	"context"
	"slices"
	"time"

	"github.com/theapemachine/symm/nomagique/learning"
)

/* LearningView is an on-demand, immutable copy for the operator dashboard. */
type LearningView struct {
	At             time.Time         `json:"at"`
	Symbol         string            `json:"symbol"`
	Status         string            `json:"status"`
	Steps          uint64            `json:"steps"`
	Decisions      uint64            `json:"decisions"`
	Resolved       uint64            `json:"resolved"`
	GridVersion    uint64            `json:"gridVersion"`
	Columns        int               `json:"columns"`
	InitialCapital string            `json:"initialCapital"`
	Universe       []LearningSummary `json:"universe"`
	Regions        []learning.Region `json:"regions"`
	Points         []LearningPoint   `json:"points"`
	Lanes          []LearningWallet  `json:"lanes"`

	/*
		Skill is the measured competence of the policy lane and the execution
		authority it justifies. Dispatched counts intents actually handed to an
		account: it is zero in a learning-only run by construction.
	*/
	Skill      SkillReading `json:"skill"`
	Dispatched uint64       `json:"dispatched"`

	/*
		Rejected counts policy intents the account did not accept, with the
		most recent reason. The agent decides from its own simulated wallet, so
		its intent and the account's actual position can disagree; that
		disagreement is reported here rather than stopping the run.
	*/
	Rejected  uint64 `json:"rejected"`
	Rejection string `json:"rejection,omitempty"`

	/*
		Execution is the account's own account of what it did with those
		intents. It is absent when the attached desk cannot report one, rather
		than filled in with zeros that would read as "nothing went wrong".
	*/
	Execution    ExecutionStatus `json:"execution"`
	HasExecution bool            `json:"hasExecution"`

	/*
		Forward is what the tape actually offered, reviewed behind real time,
		against what the policy lane was holding while it happened.
	*/
	Forward ForwardReview `json:"forward"`

	/*
		Horizon is the forward window every decision in this market is scored
		over, derived from Epochs observed impulse changes averaging EpochMean
		seconds. Until an interval has been observed the horizon is zero and
		nothing resolves.
	*/
	Horizon       time.Duration `json:"horizonNs"`
	HorizonEpochs int           `json:"horizonEpochs"`
	EpochMean     float64       `json:"epochMean"`
	Epochs        uint64        `json:"epochs"`

	/*
		Impulse is the ordered context the next decision is conditioned on,
		with each token resolved back to the quantity it names. Candidates are
		the feasible actions at that context with the evidence recalled for
		each, so a chosen action can be read against the ones it beat.
	*/
	Impulse    []LearningToken     `json:"impulse"`
	Candidates []LearningCandidate `json:"candidates"`

	/*
		Influence ranks which measured quantities have accumulated outcome
		evidence for which actions. It is association under the agent's own
		exploration, not a controlled comparison.
	*/
	Influence []MetricInfluence `json:"influence"`
}

/* LearningToken resolves one context token back to the quantity it identifies. */
type LearningToken struct {
	Token     uint64  `json:"token"`
	Source    string  `json:"source"`
	Label     string  `json:"label"`
	Strength  float64 `json:"strength"`
	Authority float64 `json:"authority"`
	Members   int     `json:"members"`
}

/*
LearningCandidate is one feasible action at the current context together with
the evidence recalled for it. Selected marks the action the policy lane would
take now; an undefined prior means this action has never completed here, which
is why exploration would reach for it.
*/
type LearningCandidate struct {
	Kind     string                `json:"kind"`
	Power    uint16                `json:"power"`
	Reduce   bool                  `json:"reduce"`
	Selected bool                  `json:"selected"`
	Prior    learning.PriorReading `json:"prior"`
}

/* LearningSummary locates active independent contexts without combining capital. */
type LearningSummary struct {
	Symbol    string `json:"symbol"`
	Status    string `json:"status"`
	Decisions uint64 `json:"decisions"`
}

/* LearningPoint exposes a cell's position and quality-conditioned activity. */
type LearningPoint struct {
	ID        uint64  `json:"id"`
	Source    string  `json:"source"`
	Label     string  `json:"label"`
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
	Value     float64 `json:"value"`
	Energy    float64 `json:"energy"`
	Authority float64 `json:"authority"`
	Present   bool    `json:"present"`
}

/* LearningWallet keeps each cloned account's actual economics separate. */
type LearningWallet struct {
	Lane       int                   `json:"lane"`
	Mode       string                `json:"mode"`
	Cash       string                `json:"cash"`
	Quantity   string                `json:"quantity"`
	Fees       string                `json:"fees"`
	Equity     float64               `json:"equity"`
	Profit     float64               `json:"profit"`
	Rate       float64               `json:"rate"`
	At         time.Time             `json:"at"`
	Complete   bool                  `json:"complete"`
	Action     LearningAction        `json:"action"`
	Pending    bool                  `json:"pending"`
	Issued     uint64                `json:"issued"`
	Fills      uint64                `json:"fills"`
	Resolved   uint64                `json:"resolved"`
	Unresolved int                   `json:"unresolved"`
	Prior      learning.PriorReading `json:"prior"`

	/*
		Episodes counts finished accounts: a lane that spent its capital on
		execution costs restarts on a fresh clone of the same known balance.
		Realized is what those finished episodes actually returned and Spent is
		what they paid in fees. Neither is a balance anyone holds, and they are
		never summed across lanes.
	*/
	Episodes  uint64  `json:"episodes"`
	Realized  float64 `json:"realized"`
	Spent     float64 `json:"spent"`
	Exhausted bool    `json:"exhausted"`
}

/* learningRequest crosses into the workspace owner; it never reads live maps. */
type learningRequest struct {
	symbol string
	reply  chan LearningView
}

/* Snapshot asks the single writer for a coherent copy, only on operator demand. */
func (agent *Agent) Snapshot(ctx context.Context, symbol string) (LearningView, error) {
	if err := ctx.Err(); err != nil {
		return LearningView{}, err
	}

	request := learningRequest{symbol: symbol, reply: make(chan LearningView, 1)}

	select {
	case agent.requests <- request:
	case <-ctx.Done():
		return LearningView{}, ctx.Err()
	case <-agent.ctx.Done():
		return LearningView{}, agent.ctx.Err()
	}

	select {
	case view := <-request.reply:
		return view, nil
	case <-ctx.Done():
		return LearningView{}, ctx.Err()
	case <-agent.ctx.Done():
		return LearningView{}, agent.ctx.Err()
	}
}

/* view runs exclusively on the workspace owner, off the ordinary hot path. */
func (agent *Agent) view(symbol string) LearningView {
	view := LearningView{At: agent.now(), Symbol: symbol, Status: "waiting for market observations",
		Steps: agent.steps, Decisions: agent.decisions, Resolved: agent.resolved,
		GridVersion: agent.Grid.Version, Columns: len(agent.Grid.Columns), InitialCapital: agent.initial.String(),
		Dispatched: agent.dispatched, Rejected: agent.rejected, HorizonEpochs: horizonEpochs,
		Influence: agent.attribution.report(agent.Grid.Columns)}

	if agent.Skill != nil {
		view.Skill = agent.Skill.Reading()
	}

	if reporter, ok := agent.Desk.(ExecutionReporter); ok && reporter != nil {
		view.Execution, view.HasExecution = reporter.Execution(), true
	}

	view.Forward = agent.forward
	view.Forward.Recent = append([]MissedOpportunity(nil), agent.forward.Recent...)

	if agent.lastRejection != nil {
		view.Rejection = agent.lastRejection.Error()
	}

	for key, market := range agent.markets {
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
	market := agent.markets[symbol]
	if market == nil {
		return view
	}
	view.Symbol, view.Status = symbol, market.status
	view.Regions = append([]learning.Region(nil), market.regions...)
	view.Horizon, view.EpochMean, view.Epochs = market.horizon(), market.epochMean, market.epochs

	for _, region := range market.regions {
		token := LearningToken{Token: region.ID, Strength: region.Strength,
			Authority: region.Authority, Members: region.Members}

		if index := int(region.ID) - 1; index >= 0 && index < len(agent.Grid.Columns) {
			token.Source, token.Label = agent.Grid.Columns[index][0], agent.Grid.Columns[index][1]
		}

		view.Impulse = append(view.Impulse, token)
	}

	// The last context and feasible set belong to the policy lane, which runs
	// last. Recall never creates evidence, so inspecting it cannot train.
	if len(market.context) > 0 {
		selected, _, err := agent.Model.Select(
			[2]string{symbol, "virtual"}, market.context, market.actions, false,
		)

		for _, candidate := range market.actions {
			view.Candidates = append(view.Candidates, LearningCandidate{
				Kind: string(candidate.Kind), Power: candidate.Power, Reduce: candidate.Reduce,
				Selected: err == nil && candidate == selected,
				Prior:    agent.Model.Recall([2]string{symbol, "virtual"}, market.context, candidate),
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

	for row, key := range agent.Grid.Rows {
		if key != symbol {
			continue
		}
		activity, quality, err := agent.Grid.Activity(symbol)
		if err != nil {
			view.Status = err.Error()
			return view
		}
		for column, identity := range agent.Grid.Columns {
			point := agent.Grid.Coordinates[column]
			view.Points = append(view.Points, LearningPoint{ID: uint64(column + 1), Source: identity[0], Label: identity[1],
				X: point[0], Y: point[1], Value: agent.Grid.Values[row][column], Energy: activity[column] * activity[column],
				Authority: quality[column], Present: agent.Grid.Present[row][column]})
		}
	}

	return view
}
