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
		GridVersion: agent.Grid.Version, Columns: len(agent.Grid.Columns), InitialCapital: agent.initial.String()}

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

	for index, lane := range market.lanes {
		mode := "virtual"
		if lane.paper {
			mode = "paper"
		}
		view.Lanes = append(view.Lanes, LearningWallet{Lane: index, Mode: mode,
			Cash: lane.wallet.cash.FloatString(lane.wallet.scale), Quantity: lane.wallet.quantity.FloatString(lane.wallet.pair.QtyPrecision), Fees: lane.wallet.fees.FloatString(lane.wallet.scale),
			Equity: lane.equity, Profit: lane.outcome.TotalReward, Rate: lane.outcome.Rate, Complete: lane.complete,
			At: lane.outcome.Through.At, Action: lane.action, Pending: lane.pending != 0,
			Issued: lane.issued, Fills: lane.fills, Resolved: lane.resolved, Unresolved: len(lane.trace), Prior: lane.lastPrior})
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
