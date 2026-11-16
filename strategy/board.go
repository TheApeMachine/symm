package strategy

import (
	"context"
	"fmt"
	"math"
	"slices"
	"sync"
	"time"

	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/system"
)

/*
Board ranks enter candidates each tick and chooses the best few for real capital.
Trade frames arrive one symbol at a time, so Place keeps a short peer-score
memory and ranks the current candidates against recent movers — otherwise every
1-of-1 slice would win MaxEntersPerTick unconditionally.
*/
type Board struct {
	*runtime.System
	rotation *Rotation
	mu       sync.Mutex
	peers    map[string]peerScore
}

type peerScore struct {
	Score float64
	At    time.Time
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
		peers:    make(map[string]peerScore),
	}
}

/*
Score ranks one symbol higher when board, ignition, tape, and rotation align.
Tape credit uses recent/prior ratio only — log1p(recent) on a dormant prior
favors sleepy wake-ups over real accelerations.
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
A lone candidate only enters when it clears MinBoardScore and ranks at the top
of the recent peer window — not merely because the trade frame was 1-of-1.
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
	now := time.Now()

	for i := range candidates {
		candidates[i].Score = board.Score(candidates[i].Symbol, envelope, price)
	}

	board.remember(candidates, now)

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

	floor := sel.MinBoardScore
	cut := board.peerCut(now, allowed)
	entered := 0

	for _, cand := range candidates {
		if entered < allowed &&
			cand.Score >= floor &&
			cand.Score >= cut {
			enter(cand)
			entered++
			continue
		}

		if cand.Decide == nil {
			continue
		}

		reason := "board rank below cut this tick"

		switch {
		case cand.Score < floor:
			reason = "board score below floor"
		case cand.Score < cut:
			reason = "board below recent peer cut"
		}

		cand.Decide(&runtime.Decision{
			Symbol: cand.Symbol,
			Action: "skip",
			Reason: fmt.Sprintf(
				"%s (score=%.3f floor=%.3f cut=%.3f)",
				reason, cand.Score, floor, cut,
			),
			Gate: "board",
			Path: cand.Path,
		})
	}
}

/*
remember merges this tick's scores into the peer window so later 1-symbol frames
still compete against recent movers.
*/
func (board *Board) remember(candidates []EnterCandidate, now time.Time) {
	board.mu.Lock()
	defer board.mu.Unlock()

	if board.peers == nil {
		board.peers = make(map[string]peerScore)
	}

	for _, cand := range candidates {
		if cand.Symbol == "" {
			continue
		}

		board.peers[cand.Symbol] = peerScore{Score: cand.Score, At: now}
	}

	board.pruneLocked(now)
}

/*
peerCut is the lowest score still inside the top-allowed ranks of the live peer
window. An empty or single-peer window returns -Inf so MinBoardScore alone gates.
*/
func (board *Board) peerCut(now time.Time, allowed int) float64 {
	board.mu.Lock()
	defer board.mu.Unlock()

	board.pruneLocked(now)

	if allowed <= 0 || len(board.peers) <= allowed {
		return math.Inf(-1)
	}

	scores := make([]float64, 0, len(board.peers))

	for _, peer := range board.peers {
		scores = append(scores, peer.Score)
	}

	slices.SortFunc(scores, func(a, b float64) int {
		switch {
		case a > b:
			return -1
		case a < b:
			return 1
		default:
			return 0
		}
	})

	return scores[allowed-1]
}

func (board *Board) pruneLocked(now time.Time) {
	window := system.Cfg.Selection.BoardPeerWindow

	if window <= 0 {
		window = 2 * time.Second
	}

	for symbol, peer := range board.peers {
		if now.Sub(peer.At) > window {
			delete(board.peers, symbol)
		}
	}
}
