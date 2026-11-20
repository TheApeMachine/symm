package strategy

import (
	"context"
	"fmt"
	"math"
	"slices"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/system"
)

/*
Board is cross-sectional enter capacity on top of per-symbol gates (oracle,
resonance, story, tree). It ranks candidates from the tick envelope — grid
pipeline outputs — and the shared price book, then applies MaxEntersPerTick
and real open caps. Peer memory lets single-symbol frames compete with recent
movers instead of auto-winning a 1-of-1 slice. Every enter uses the same
sizing and slot rules — hold duration is an outcome, not a lane.
*/
type Board struct {
	*runtime.System
	mu    sync.Mutex
	peers map[string]peerScore
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
	PathFound  bool
	Score      float64
	Tier       string
}

/*
NewBoard starts the cross-sectional ranker for model enter candidates.
*/
func NewBoard(ctx context.Context) *Board {
	return &Board{
		System: runtime.NewSystem(ctx, "board"),
		peers:  make(map[string]peerScore),
	}
}

/*
Score ranks one symbol from envelope readings and price tape/spread/peak.
Tape credit uses double-conditioned ratio, net buy pressure, rush, and rhythm
so dormant wake-ups and wash churn cannot outrank real accelerations.
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

	if lag := envelope.LeadLag[symbol]; lag != nil {
		score += sel.WeightSync * lag.SyncScore
		score -= sel.WeightSync * 0.5 * lag.DecoupledScore
	}

	if sent := envelope.Sentiment[symbol]; sent != nil {
		score += sel.WeightSurge * sent.SurgeScore
		score -= sel.WeightSurge * 0.5 * sent.SlumpScore
	}

	if liq := envelope.Liquidity[symbol]; liq != nil {
		score += sel.WeightDepth * liq.DepthScore
		score -= sel.WeightDepth * 0.5 * liq.ScarcityScore
	}

	if cvd := envelope.CVD[symbol]; cvd != nil {
		score += sel.WeightDepth * 0.35 * cvd.NetFraction
		score -= sel.WeightDepth * 0.25 * cvd.Balance
	}

	if depth := envelope.DepthFlow[symbol]; depth != nil {
		// Bolz: book-pressure z ahead of trade spike.
		score += sel.WeightDepth * 0.40 * math.Tanh(depth.PressureZ/2)
		// Cont OFI: contemporaneous liquidity shift — soft entry cue only;
		// exit pacing consumes the same reading when already green.
		score += sel.WeightDepth * 0.15 * math.Tanh(depth.EventOFI)
	}

	if hawkes := envelope.Hawkes[symbol]; hawkes != nil {
		// Filimonov branching ratio: subcritical heat lifts score; near-critical
		// is entry caution (exit pacing consumes the same reading when green).
		radius := hawkes.Fit.SpectralRadius
		if radius > 0 && radius < 0.95 {
			score += sel.WeightRotation * 0.5 * radius
		} else if radius >= 0.95 {
			score -= sel.WeightRotation * (radius - 0.9)
		}
	}

	if price != nil {
		risk := system.Cfg.Risk
		flow := price.TapeFlow(symbol, time.Now())
		floor := risk.MinTapeAccelRatio
		if floor <= 0 {
			floor = 1
		}

		ratio := flow.ConditionedRatio
		if ratio >= floor {
			score += sel.WeightTapeUptake * math.Log1p(ratio)
		} else if ratio > 0 {
			score -= sel.WeightTapeUptake * (floor - ratio)
		}

		if flow.NetFraction < 0 {
			score += sel.WeightTapeUptake * flow.NetFraction
		}

		if risk.MaxFlipChurn > 0 && flow.FlipChurn > risk.MaxFlipChurn {
			score -= sel.WeightTapeUptake * math.Log1p(flow.FlipChurn/risk.MaxFlipChurn)
		}

		if sel.WeightRushBuy > 0 && flow.RushBuy > 0 {
			rush := flow.RushBuy
			if flow.RushSell > 0 {
				rush -= flow.RushSell
			}
			if rush > 0 {
				score += sel.WeightRushBuy * math.Log1p(rush/
					math.Max(risk.MinTapeNotionalUSD, 1))
			} else {
				score += sel.WeightRushBuy * rush /
					math.Max(risk.MinTapeNotionalUSD, 1)
			}
		}

		if sel.WeightRhythm > 0 {
			score += sel.WeightRhythm * (flow.Rhythm - 0.5)
		}

		if overhead, ok := price.OverheadRatio(symbol); ok {
			minOver := risk.MinOverheadRatio
			if minOver > 0 && overhead < minOver {
				score -= sel.WeightDepth * (minOver - overhead)
			}
		}

		if spread, ok := price.SpreadPct(symbol); ok && spread != nil {
			score -= sel.WeightSpread * (spread.InexactFloat64() / 100)
		}

		if frac, ok := price.PeakFrac(symbol); ok {
			score -= sel.WeightPeakFrac * frac
		}
	}

	if rez := envelope.Resonance[symbol]; rez != nil && rez["calibrated"] >= 1 {
		score += sel.WeightRotation * rez["task_skill"]
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
		board.Error(errnie.Err(
			errnie.Validation,
			"[board] selection.max_enters_per_tick required",
			nil,
		))

		return
	}

	allowed := limit

	if live && desk != nil && sel.MaxOpenPositions > 0 {
		open := desk.OpenCount(broker.TierTypeReal)
		workingSlots := sel.MaxOpenPositions - open

		if workingSlots <= 0 {
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
					Tier:   cand.Tier,
				})
			}

			return
		}

		if workingSlots < allowed {
			allowed = workingSlots
		}
	}

	floor := sel.MinBoardScore
	cut := board.peerCut(now, limit)
	entered := 0

	for _, cand := range candidates {
		room := entered < allowed

		if room && cand.Score >= floor && cand.Score >= cut {
			entered++
			enter(cand)
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
		case !room:
			reason = "board cap — open slots full"
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
			Tier: cand.Tier,
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
