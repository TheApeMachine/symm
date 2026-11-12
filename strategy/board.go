package strategy

import (
	"context"
	"math"
	"slices"
	"time"

	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/system"
)

/*
Board ranks enter candidates each tick and chooses the best few for real capital.
*/
type Board struct {
	*runtime.System
	rotation *Rotation
}

/*
EnterCandidate is one symbol the model would enter before cross-sectional gating.
*/
type EnterCandidate struct {
	Decide     func(*runtime.Decision)
	Symbol     string
	Path       string
	Resistance float64
	Bypass     bool
	PathFound  bool
	Score      float64
}

/*
NewBoard composes rotation memory into enter ranking.
*/
func NewBoard(ctx context.Context, rotation *Rotation) *Board {
	return &Board{
		System:   runtime.NewSystem(ctx, "board"),
		rotation: rotation,
	}
}

/*
Score ranks one symbol higher when board, ignition, tape, and rotation align.
*/
func (board *Board) Score(
	symbol string,
	envelope *runtime.Envelope,
	price *broker.Price,
) float64 {
	if board == nil || symbol == "" || envelope == nil {
		return math.Inf(-1)
	}

	sel := system.Cfg.Selection
	score := 0.0

	if corr := envelope.Correlation[symbol]; corr != nil {
		score += sel.WeightPeakScore * corr.PeakScore
		score += sel.WeightPeakScore * 0.5 * corr.HerdScore
	}

	if pump := envelope.Pumpdump[symbol]; pump != nil {
		score += sel.WeightPrecursor * pump.Precursor
		score += sel.WeightPrecursor * 0.5 * pump.Compression
		score += sel.WeightPrecursor * 0.25 * pump.Ignition
	}

	if price != nil {
		recent, prior := price.TapeUptake(symbol, time.Now())

		if prior > 0 {
			score += sel.WeightTapeUptake * math.Log1p(recent/prior)
		} else if recent > 0 {
			score += sel.WeightTapeUptake * math.Log1p(recent)
		}

		if spread, ok := price.SpreadPct(symbol); ok && spread != nil {
			score -= sel.WeightSpread * spread.InexactFloat64()
		}

		if frac, ok := price.PeakFrac(symbol); ok {
			score -= sel.WeightPeakFrac * frac
		}
	}

	if board.rotation != nil {
		score += sel.WeightRotation * board.rotation.Score(symbol)
	}

	return score
}

/*
Place scores, caps concurrent opens, and dispatches desk enters for winners.
*/
func (board *Board) Place(
	candidates []EnterCandidate,
	envelope *runtime.Envelope,
	price *broker.Price,
	desk *broker.Desk,
	live bool,
	enter func(EnterCandidate),
) {
	if board == nil || len(candidates) == 0 || enter == nil {
		return
	}

	sel := system.Cfg.Selection

	for i := range candidates {
		candidates[i].Score = board.Score(candidates[i].Symbol, envelope, price)
	}

	slices.SortFunc(candidates, func(a, b EnterCandidate) int {
		switch {
		case a.Score > b.Score:
			return -1
		case a.Score < b.Score:
			return 1
		default:
			return 0
		}
	})

	limit := sel.MaxEntersPerTick

	if limit <= 0 {
		limit = 1
	}

	allowed := limit

	if live && desk != nil && sel.MaxOpenPositions > 0 {
		open := desk.OpenCount(broker.TierTypeReal)
		slots := sel.MaxOpenPositions - open

		if slots <= 0 {
			for _, cand := range candidates {
				if cand.Decide == nil {
					continue
				}

				cand.Decide(&runtime.Decision{
					Symbol: cand.Symbol,
					Action: "skip",
					Reason: "board cap — max open positions",
					Gate:   "board",
					Path:   cand.Path,
				})
			}

			return
		}

		if slots < allowed {
			allowed = slots
		}
	}

	entered := 0

	for _, cand := range candidates {
		if entered < allowed {
			enter(cand)
			entered++
			continue
		}

		if cand.Decide == nil {
			continue
		}

		cand.Decide(&runtime.Decision{
			Symbol: cand.Symbol,
			Action: "skip",
			Reason: "board rank below cut this tick",
			Gate:   "board",
			Path:   cand.Path,
		})
	}
}
