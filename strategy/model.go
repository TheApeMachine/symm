package strategy

import (
	"context"
	"math"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/nomagique/store"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/logic/resonance"
	"github.com/theapemachine/symm/system"
)

/*
Model turns each tick's path cut into desk enter/hold/exit decisions and only
mutates the resistance tree on settled positions — fills and gates stay ahead of
learning so bad sends do not train the path book.
*/
type Model struct {
	*runtime.System
	desk      *broker.Desk
	price     *broker.Price
	store     *store.Client
	tree      *atomic.Pointer[Tree]
	board     *Board
	resetPath func(string)
}

/*
NewModel swaps the path tree via atomic.Pointer so S3 reload and Step never
race on the same radix; resetPath clears grammar when tree actions consume a cut.
*/
func NewModel(
	ctx context.Context,
	desk *broker.Desk,
	price *broker.Price,
	resetPath func(string),
	client *store.Client,
) *Model {
	board := NewBoard(ctx)

	model := &Model{
		System:    runtime.NewSystem(ctx, "model"),
		desk:      desk,
		price:     price,
		store:     client,
		tree:      &atomic.Pointer[Tree]{},
		board:     board,
		resetPath: resetPath,
	}

	tree := NewTree(ctx, "model/paths/", client)
	model.tree.Store(tree)

	if err := errnie.Require(map[string]any{
		"tree": tree,
	}); err != nil {
		errnie.Error(err)
	}

	return model
}

/*
Step drains stoploss pending exits first, then decides per symbol in parallel:
oracle/resonance/story gates, tree Predict, tier routing — decisions land on
the envelope for cmd and the hub without calling the broker on blocked paths.
*/
func (model *Model) Step(envelope *runtime.Envelope) *runtime.Envelope {
	if envelope == nil {
		errnie.Error(errnie.Err(
			errnie.NotFound,
			"[strategy.model.Step] envelope is nil",
			nil,
		))

		return nil
	}

	// First we need to handle all pending exits, which are positions
	// that have exited via the Stoploss mechanism.
	for _, tier := range []broker.TierType{
		broker.TierTypeVirtual, broker.TierTypePaper, broker.TierTypeReal,
	} {
		for position := range model.desk.Pending(tier) {
			position.ExitPath = envelope.Paths[position.Symbol].String()
			errnie.Error(model.desk.Exit(position, false))
		}
	}

	var wg sync.WaitGroup

	for symbol, cut := range envelope.Paths {
		if cut == nil || cut.Len() < system.Cfg.Learning.MinPathLength {
			continue
		}

		wg.Go(func() {
			path := cut.String()
			open := model.desk.Has(symbol)

			// Oracle and resonance gates are entry filters. An open book still
			// has to hold or exit when the reading is cold or surprising.
			if !open && !model.oracleAllows(envelope.Regions[symbol]) {
				envelope.Decide(&runtime.Decision{
					Symbol: symbol,
					Action: "skip",
					Reason: "oracle veto lit — not tradable",
					Gate:   "oracle",
					Path:   path,
				})

				return
			}

			rez := envelope.Resonance[symbol]

			if rez == nil && !open {
				envelope.Decide(&runtime.Decision{
					Symbol: symbol,
					Action: "skip",
					Reason: "no resonance reading",
					Gate:   "resonance",
					Path:   path,
				})
				return
			}

			var (
				surprise     float64
				supported    int
				precision    float64
				hasPrecision bool
			)

			if rez != nil {
				surprise = rez["surprise"]
				supported = int(rez["supported_horizon"])
				precision, hasPrecision = rez["task_relative_precision"]
			}

			learn := system.Cfg.Learning

			if rez != nil && !open && (rez["calibrated"] < 1 ||
				supported < learn.MinSupportedHorizon ||
				!hasPrecision ||
				precision < learn.MinTaskRelativePrecision ||
				surprise < learn.MinSurprise) {
				envelope.Decide(&runtime.Decision{
					Symbol: symbol,
					Action: "skip",
					Reason: model.gateReason(rez, supported, hasPrecision, precision, surprise),
					Gate:   "resonance",
					Path:   path,
				})
				return
			}

			curve := make([]float64, 0, len(resonance.Features))

			if rez != nil {
				for i := range len(resonance.Features) {
					value, ok := rez["forward_curve_"+strconv.Itoa(i)]
					if !ok {
						break
					}

					curve = append(curve, value)
				}
			}

			enterNet, holdNet, exitNet, risk, ok := resonance.ForwardStoryNets(curve)

			if !ok && !open {
				envelope.Decide(&runtime.Decision{
					Symbol: symbol,
					Action: "skip",
					Reason: "resonance curve incomplete",
					Gate:   "resonance",
					Path:   path,
				})

				return
			}

			minEnter := system.Cfg.Learning.MinEnterZ
			bullish := (enterNet >= minEnter && holdNet >= minEnter && exitNet <= holdNet && risk < minEnter)

			if !bullish {
				envelope.Decide(&runtime.Decision{
					Symbol: symbol,
					Action: "skip",
					Reason: "forward story not bullish",
					Gate:   "story",
					Path:   path,
				})
			}

			tree := model.tree.Load()
			action, resistance, matched := tree.Predict(*cut)

			tier := broker.TierTypeVirtual

			// Matched + ActionNone: leaf existed but brightness lost to resistance
			// (or the leaf was pruned). That is a veto, not an unexplored cold path.
			if matched {
				if resistance >= system.Cfg.Learning.PaperEligibleResistance {
					tier = broker.TierTypePaper
				}

				position := model.desk.Position(tier, symbol)

				if position != nil {
					position.HoldPaths = append(position.HoldPaths, path)
				}

				switch action {
				case ActionNone:
					if open && path != "" {
						envelope.Decide(&runtime.Decision{
							Symbol: symbol,
							Action: "hold",
							Reason: "path resistance veto",
							Gate:   "tree",
							Path:   path,
						})
					}
				case ActionEnter:
					model.desk.Enter(symbol, path, tier, true)

					envelope.Decide(&runtime.Decision{
						Symbol: symbol,
						Action: "enter",
						Reason: "path matched",
						Gate:   "tree",
						Path:   path,
					})
				case ActionExit:
					envelope.Decide(&runtime.Decision{
						Symbol: symbol,
						Action: "exit",
						Reason: "path matched",
						Gate:   "tree",
						Path:   path,
					})
				}

				return
			}

			model.desk.Enter(symbol, path, broker.TierTypeVirtual, false)
		})
	}

	wg.Wait()

	return envelope
}

/*
oracleAllows is true when at least one approve is lit and no lit veto is
selected. Grid.Observe emits exactly one side of each oracle pair; desk bool
vetoes are hard brightness 1. Idle metric ties (board / contagion / capacity
at brightness 0 while kernels are cold) stay out of the way — only a lit
veto fails closed.
*/
func (model *Model) oracleAllows(regions []*runtime.Region) bool {
	approved := false

	for _, region := range regions {
		if region == nil {
			continue
		}

		switch store.Catalog.ByKey(region.Key).Side {
		case "veto":
			if region.Brightness > 0 {
				return false
			}
		case "approve":
			if region.Brightness > 0 {
				approved = true
			}
		}
	}

	return approved
}

/*
gateReason builds human skip text for resonance vetoes — UI and replay need which
threshold failed without re-deriving curve math from raw envelope fields.
*/
func (model *Model) gateReason(
	rez map[string]float64,
	supported int,
	hasPrecision bool,
	precision, surprise float64,
) string {
	learn := system.Cfg.Learning

	if rez["calibrated"] < 1 {
		return "resonance not calibrated"
	}

	if supported < learn.MinSupportedHorizon {
		return "supported horizon below minimum"
	}

	if !hasPrecision {
		return "task-relative precision not ready"
	}

	if precision < learn.MinTaskRelativePrecision {
		return "task-relative precision below threshold"
	}

	if surprise < learn.MinSurprise {
		return "surprise below threshold"
	}

	return "resonance gate"
}

/*
Exit arms broker sells before tree Update on pending positions — resistance moves
only after ClosedAt so rejects and repeated marks cannot train on ghost exits.
*/
func (model *Model) Exit(position *broker.Position, path string) {
	if position == nil {
		return
	}

	if position.Status() == runtime.BUSY || position.ClosedAt != nil {
		return
	}

	position.ExitPath = path

	if position.ExitPath == "" && len(position.HoldPaths) > 1 {
		position.ExitPath = strings.Join(
			position.HoldPaths[len(position.HoldPaths)-2:], "/",
		)

		return
	}

	if position.ExitPath == "" {
		return
	}

	model.tree.Load().Update(
		position.ExitPath, model.resistanceDelta(position),
	)
}

/*
resistanceDelta is the leaf step for a settled (or settling) position.

Losses raise resistance by the full ThresholdStep. Wins lower it by
step × timeWeight × pnlWeight, where short holds and larger fee-net PnL%
earn more credit, never more than one step per proof. Hold duration is
measured in seconds against a multi-hour half-life (positions live minutes–hours).
*/
func (model *Model) resistanceDelta(position *broker.Position) float64 {
	step := system.Cfg.Learning.ThresholdStep

	if position == nil || position.PnL == nil || position.PnL.Sign() <= 0 {
		return step
	}

	halfLife := system.Cfg.Learning.ResistanceHoldHalfLifeSec
	pnlRef := system.Cfg.Learning.ResistancePnLRefPct

	hold := model.holdDuration(position, halfLife).Seconds()
	timeWeight := 1.0 / (1.0 + hold/halfLife)

	pnlPct := 0.0

	if position.PnLPct != nil {
		pnlPct = position.PnLPct.InexactFloat64()
	}

	pnlWeight := math.Tanh(pnlPct / pnlRef)

	if pnlWeight < 0 {
		pnlWeight = 0
	}

	return -step * timeWeight * pnlWeight
}

/*
holdDuration clamps negative or missing clocks so resistance credit for quick
wins does not explode when OpenedAt/ClosedAt are partial on exchange imports.
*/
func (model *Model) holdDuration(position *broker.Position, halfLifeSec float64) time.Duration {
	if position.OpenedAt.IsZero() {
		return time.Duration(halfLifeSec * float64(time.Second))
	}

	end := time.Now().UTC()

	if position.ClosedAt != nil {
		end = position.ClosedAt.UTC()
	}

	hold := end.Sub(position.OpenedAt.UTC())

	if hold < 0 {
		return time.Duration(halfLifeSec * float64(time.Second))
	}

	return hold
}
