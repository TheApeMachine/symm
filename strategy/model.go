package strategy

import (
	"context"
	"strings"
	"sync/atomic"

	"github.com/theapemachine/nomagique/data"
	"github.com/theapemachine/nomagique/runtime"
	nmstore "github.com/theapemachine/nomagique/store"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/strategy/tokenizer"
	"github.com/theapemachine/symm/system"
)

type Model struct {
	*runtime.System
	desk     *broker.Desk
	price    *broker.Price
	store    *store.Client
	tree     *atomic.Pointer[Tree]
	stage    *Stage
	rank     *Rank
	ignition *Ignition
}

func NewModel(
	ctx context.Context,
	desk *broker.Desk,
	price *broker.Price,
	stage *Stage,
) *Model {
	client := store.NewClient(ctx, system.Cfg.Storage.S3.Bucket)

	model := &Model{
		System:   runtime.NewSystem(ctx, "model"),
		desk:     desk,
		price:    price,
		store:    client,
		tree:     &atomic.Pointer[Tree]{},
		stage:    stage,
		rank:     NewRank(),
		ignition: NewIgnition(),
	}

	tree := NewTree(ctx, "model/paths/")
	model.tree.Store(tree)

	return model
}

func (model *Model) Evaluate(
	out *data.Value, observed []*nmstore.Region, tick *data.Value,
) *data.Value {
	if out == nil {
		return nil
	}

	symbol := out.Scope

	if tick == nil {
		return out
	}

	tokens := data.Get[tokenizer.Tokens](
		tick, strings.Join([]string{"sequencer", symbol, "cut"}, "/"),
	)

	path := tokens.String()

	out.Append(tokens, "cut")

	// Settled lots must land in the tree even when the coder is cold: the
	// round trip already happened, and gating here left pending forever.
	if pending, ok := model.desk.TakePending(symbol); ok {
		model.Exit(pending, path)
	}

	coder := ReadCoder(tick, symbol)

	if !coder.Ready {
		if tokens.Len() > 0 {
			auditHold(symbol, "coder cold", path, coder)
		}

		return out
	}

	out.Append(observed, "lit")

	position := model.desk.Position(symbol)

	if position != nil && position.IsClosed() {
		position = nil
	}

	regime := FlatRegime()

	if position != nil {
		regime = OpenRegime()
	}

	action, strength, disagreed := model.tree.Load().Predict(tokens, regime)

	if disagreed {
		if tokens.Len() > 0 {
			auditHold(symbol, "tree disagreed", path, coder)
		}

		return out
	}

	if action != ActionNone {
		switch action {
		case ActionEnter:
			if coder.Bullish && position == nil {
				paper := strength > 0.2
				model.offerEnter(symbol, path, !paper, paper, tick, observed)
			} else if tokens.Len() > 0 {
				reason := "tree enter held"
				if !coder.Bullish {
					reason = "tree enter but not bullish"
				} else if position != nil {
					reason = "tree enter but already open"
				}

				auditHold(symbol, reason, path, coder)
			}
		case ActionExit:
			if !coder.Bullish && position != nil {
				model.Exit(position, path)
			}

			model.stage.ResetSequencer(symbol)
		case ActionHold:
			if position != nil {
				model.Hold(position, path)
			}

			model.stage.ResetSequencer(symbol)
		}

		return out
	}

	minPath := system.Cfg.Learning.MinPathLength

	// While the lot is open, each new cut (after entry) is a hold segment for
	// learning — same idea as entry/exit paths, not gated on trie hold leaves.
	if position != nil && path != "" && action != ActionExit {
		if minPath <= 0 || tokens.Len() >= minPath {
			model.Hold(position, path)
		}
	}

	if !coder.Bullish || position != nil || tokens.Len() == 0 {
		if tokens.Len() > 0 && position == nil && !coder.Bullish {
			auditHold(symbol, "fallthrough not bullish", path, coder)
		}

		return out
	}

	if minPath > 0 && tokens.Len() < minPath {
		auditHold(symbol, "cut shorter than min path length", path, coder)
		return out
	}

	model.offerEnter(symbol, path, true, false, tick, observed)
	return out
}

func (model *Model) offerEnter(
	symbol, path string,
	virtual, pathFound bool,
	tick *data.Value,
	observed []*nmstore.Region,
) {
	if model == nil {
		return
	}

	spreadPct := 0.0

	if model.price != nil {
		if spread, ok := model.price.SpreadPct(symbol); ok && spread != nil {
			spreadPct = spread.Float64()
		}
	}

	accel := clockAcceleration(tick, symbol)
	brightness := peakBrightness(observed)
	score := Score(accel, brightness, spreadPct)

	if !model.ignition.Clears(accel) || score <= 0 {
		audit.Record.Write(map[string]any{
			"node":       "entry",
			"phase":      "ranked",
			"symbol":     symbol,
			"path":       path,
			"virtual":    virtual,
			"pathFound":  pathFound,
			"score":      score,
			"accel":      accel,
			"brightness": brightness,
			"spreadPct":  spreadPct,
			"ok":         false,
			"reason":     "ignition or score below floor",
		})
		return
	}

	if model.rank == nil {
		err := model.Enter(symbol, path, virtual, pathFound)

		record := map[string]any{
			"node":      "entry",
			"phase":     "commit",
			"symbol":    symbol,
			"path":      path,
			"virtual":   virtual,
			"pathFound": pathFound,
			"ok":        err == nil,
		}

		if err != nil {
			record["reason"] = err.Error()
		}

		audit.Record.Write(record)
		model.stage.ResetSequencer(symbol)
		return
	}

	model.rank.Offer(Opportunity{
		Symbol:    symbol,
		Path:      path,
		Virtual:   virtual,
		PathFound: pathFound,
		Score:     score,
	})

	audit.Record.Write(map[string]any{
		"node":       "entry",
		"phase":      "ranked",
		"symbol":     symbol,
		"path":       path,
		"virtual":    virtual,
		"pathFound":  pathFound,
		"score":      score,
		"accel":      accel,
		"brightness": brightness,
		"spreadPct":  spreadPct,
	})
}

/*
CommitEntries opens ranked opportunities best-first so cash goes to the
highest (accel × brightness / spread) scores this tick.
*/
func (model *Model) CommitEntries() {
	if model == nil || model.rank == nil {
		return
	}

	for _, opp := range model.rank.TakeSorted() {
		err := model.Enter(opp.Symbol, opp.Path, opp.Virtual, opp.PathFound)

		record := map[string]any{
			"node":      "entry",
			"phase":     "commit",
			"symbol":    opp.Symbol,
			"path":      opp.Path,
			"virtual":   opp.Virtual,
			"pathFound": opp.PathFound,
			"score":     opp.Score,
			"ok":        err == nil,
		}

		if err != nil {
			record["reason"] = err.Error()
		}

		audit.Record.Write(record)
		model.stage.ResetSequencer(opp.Symbol)
	}
}

func (model *Model) Enter(
	symbol string,
	path string,
	virtual bool,
	pathFound bool,
) error {
	return model.desk.Enter(symbol, path, virtual, pathFound)
}

func (model *Model) Hold(position *broker.Position, path string) {
	if position == nil || path == "" || path == position.EntryPath {
		return
	}

	n := len(position.HoldPaths)

	if n > 0 && position.HoldPaths[n-1] == path {
		return
	}

	position.HoldPaths = append(position.HoldPaths, path)
}

func (model *Model) Exit(
	position *broker.Position, path string,
) {
	if position == nil {
		return
	}

	holdPaths := position.HoldPaths

	// Empty cut: use the latest hold spell, never EntryPath.
	if path == "" {
		if len(holdPaths) > 0 {
			path, holdPaths = holdPaths[len(holdPaths)-1], holdPaths[:len(holdPaths)-1]
		}
	}

	if path != "" {
		position.ExitPath = path
	}

	tree := model.tree.Load()

	if tree == nil {
		return
	}

	if position.Virtual {
		tree.Insert(
			position.EntryPath,
			data.NewValue("model", "action", "enter"),
		)

		for _, hold := range holdPaths {
			tree.Insert(hold, data.NewValue("model", "action", "hold"))
		}

		tree.Insert(
			position.ExitPath,
			data.NewValue("model", "action", "exit"),
		)
	}

	resistance := 0.05

	if position.PnL != nil && position.PnL.Sign() > 0 {
		resistance = -0.05
	}

	tree.Update(position.EntryPath, resistance)

	for _, hold := range position.HoldPaths {
		tree.Update(hold, resistance)
	}

	tree.Update(position.ExitPath, resistance)
}

func auditHold(symbol, reason, path string, coder CoderReading) {
	audit.Record.Write(map[string]any{
		"node":       "entry",
		"phase":      "hold",
		"symbol":     symbol,
		"reason":     reason,
		"path":       path,
		"bullish":    coder.Bullish,
		"ready":      coder.Ready,
		"peakCurve":  coder.PeakCurve,
		"enterNet":   coder.EnterNet,
		"holdNet":    coder.HoldNet,
		"exitNet":    coder.ExitNet,
		"risk":       coder.Risk,
		"confidence": coder.Confidence,
	})
}
