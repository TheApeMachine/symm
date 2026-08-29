package types

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/learning"
)

const (
	TriggerHardFloor         = "hard_floor"
	TriggerProtectedFloor    = "protected_floor"
	TriggerProfitStagnation  = "profit_stagnation"
	TriggerPumpMomentumLost  = "pump_momentum_exhausted"
	TriggerTrailingFloor     = "trailing_floor"
	TriggerHorizonExpired    = "horizon_expired"
	TriggerRegimeInvalidated = "execution_regime_invalidated"
	TriggerManualOverride    = "manual_override"
)

/*
Stoploss regulates one open lot across discovery, protected profit, trailing, and
fast-surge regimes.

The first executable mark at or below Floor triggers immediately. Protection starts
as soon as a new peak can place its trailing floor above ProfitLine, then every new
peak ratchets that floor upward without ever lowering it.

Every giveback tolerance is stated in the run's own learned units. The lot keeps a
running distribution of the positive steps its peaks advanced by, and the trail,
stagnation, and surge boundaries are derived from that distribution, so a position
that has run far from entry is judged against the noise of the run, not against an
absolute distance measured when the lot was opened — a trail frozen at entry gets
proportionally tighter as price rises and shakes winners out on ordinary breathing.

A statistically unusual profitable acceleration arms a tighter regime: the armed lot
carries a live line one central band (mean plus one sigma) below its peak, so a
burst that unwinds as fast as it formed is exited before the slower trailing floor
is reached, while a burst that consolidates is still given the room its own step
distribution says is ordinary.
*/
type Stoploss struct {
	ctx                  context.Context     `json:"-"`
	cancel               context.CancelFunc  `json:"-"`
	forecast             *learning.RLSOutput `json:"-"`
	TickSize             *decimal.Decimal    `json:"tick_size,omitempty"`
	EntryFeeRate         *decimal.Decimal    `json:"entry_fee_rate,omitempty"`
	ExitFeeRate          *decimal.Decimal    `json:"exit_fee_rate,omitempty"`
	RiskDistance         *decimal.Decimal    `json:"risk_distance,omitempty"`
	TrailDistance        *decimal.Decimal    `json:"trail_distance,omitempty"`
	ArmBuffer            *decimal.Decimal    `json:"arm_buffer,omitempty"`
	LockBuffer           *decimal.Decimal    `json:"lock_buffer,omitempty"`
	MinEdge              *decimal.Decimal    `json:"min_edge,omitempty"`
	NoiseBand            *decimal.Decimal    `json:"noise_band,omitempty"`
	ConfirmMarks         int                 `json:"confirm_marks,omitempty"`
	DistinctNonPeakMarks int                 `json:"distinct_non_peak_marks,omitempty"`
	LastStagnationMark   *decimal.Decimal    `json:"last_stagnation_mark,omitempty"`
	ProfitLatched        bool                `json:"profit_latched,omitempty"`
	PositiveMoveCount    int                 `json:"positive_move_count,omitempty"`
	PositiveMoveMean     float64             `json:"positive_move_mean,omitempty"`
	PositiveMoveM2       float64             `json:"positive_move_m2,omitempty"`
	Horizon              int                 `json:"horizon,omitempty"`
	Observed             int                 `json:"observed,omitempty"`
	ClockArmed           bool                `json:"clock_armed,omitempty"`
	// BookObserved latches true once a valid authoritative L3 book has been
	// read after the stop became live. It distinguishes clean initial
	// bootstrap (no book yet, stay armed and wait) from feed-integrity failure
	// after protection was live (a valid book that becomes invalid owns an
	// execution-regime invalidation).
	BookObserved         bool                `json:"book_observed,omitempty"`
	Status               Status              `json:"status"`
	Symbol               string              `json:"symbol"`
	/*
		EntryAt identifies which specific lot this stored state belongs to.
		The position_stoplosses table is keyed by symbol, and a symbol is
		re-entered many times over a session — without this, a stored row
		from an already-closed trade is indistinguishable from the row for
		the position currently open on the same symbol, and recovery could
		silently restore a position with another trade's stale floor,
		trigger status, and geometry.
	*/
	EntryAt              *time.Time          `json:"entry_at,omitempty"`
	Floor                *decimal.Decimal    `json:"floor,omitempty"`
	Mark                 *decimal.Decimal    `json:"mark,omitempty"`
	Peak                 *decimal.Decimal    `json:"peak,omitempty"`
	ProfitLine           *decimal.Decimal    `json:"profit_line,omitempty"`
	ArmAt                *decimal.Decimal    `json:"arm_at,omitempty"`
	LockFloor            *decimal.Decimal    `json:"lock_floor,omitempty"`
	Locked               bool                `json:"locked,omitempty"`
	TriggerReason        string              `json:"trigger_reason,omitempty"`
	TriggerMark          *decimal.Decimal    `json:"trigger_mark,omitempty"`
	SurgeArmed           bool                `json:"surge_armed,omitempty"`
	LastMove             *decimal.Decimal    `json:"last_move,omitempty"`
	SurgeMove            *decimal.Decimal    `json:"surge_move,omitempty"`
	MomentumFloor        *decimal.Decimal    `json:"momentum_floor,omitempty"`
	Plan                 *RiskPlan           `json:"plan,omitempty"`
}

/*
NewStoploss constructs the entry regulator from the admitted forecast and venue facts.
*/
func NewStoploss(
	ctx context.Context,
	symbol string,
	entryPrice *decimal.Decimal,
	mark *decimal.Decimal,
	forecast *learning.RLSOutput,
	forwardCurve []float64,
	tickSize *decimal.Decimal,
	entryFeeRate *decimal.Decimal,
	exitFeeRate *decimal.Decimal,
	entryAt time.Time,
) (*Stoploss, error) {
	if symbol == "" {
		return nil, fmt.Errorf("stoploss: symbol required")
	}

	ctx, cancel := context.WithCancel(ctx)
	stoploss := &Stoploss{
		ctx:          ctx,
		cancel:       cancel,
		forecast:     forecast,
		TickSize:     tickSize,
		EntryFeeRate: entryFeeRate,
		ExitFeeRate:  exitFeeRate,
		Status:       ARMED,
		Symbol:       symbol,
		Horizon:      len(forwardCurve),
		ConfirmMarks: 3,
		MinEdge:      tickSize,
	}

	floor, trailDistance, err := stoploss.forecastGeometry(mark, forwardCurve)

	if err != nil {
		cancel()
		return nil, err
	}

	stoploss.Mark = mark
	stoploss.Peak = mark
	stoploss.Floor = floor
	stoploss.TrailDistance = trailDistance
	stoploss.RiskDistance = trailDistance
	stoploss.NoiseBand = trailDistance

	if err := stoploss.RebindFill(entryPrice, mark, entryAt); err != nil {
		cancel()
		return nil, err
	}

	return stoploss, nil
}

/*
NewStoplossWithPlan constructs a regulator with explicit RiskPlan geometry and supported horizon.
*/
func NewStoplossWithPlan(
	ctx context.Context,
	symbol string,
	entryPrice *decimal.Decimal,
	mark *decimal.Decimal,
	forecast *learning.RLSOutput,
	horizon int,
	tickSize *decimal.Decimal,
	entryFeeRate *decimal.Decimal,
	exitFeeRate *decimal.Decimal,
	plan *RiskPlan,
	entryAt time.Time,
) (*Stoploss, error) {
	if symbol == "" {
		return nil, fmt.Errorf("stoploss: symbol required")
	}

	ctx, cancel := context.WithCancel(ctx)
	stoploss := &Stoploss{
		ctx:          ctx,
		cancel:       cancel,
		forecast:     forecast,
		TickSize:     tickSize,
		EntryFeeRate: entryFeeRate,
		ExitFeeRate:  exitFeeRate,
		Status:       ARMED,
		Symbol:       symbol,
		Horizon:      horizon,
		ConfirmMarks: 3,
		MinEdge:      tickSize,
	}

	if plan != nil && plan.Present {
		stoploss.SetRiskPlan(*plan)

		if entryPrice != nil && entryPrice.Sign() > 0 &&
			stoploss.RiskDistance != nil && stoploss.RiskDistance.Sign() > 0 {
			stoploss.Floor = floorToTick(
				scaled(entryPrice).Sub(stoploss.RiskDistance),
				tickSize,
			)
		}
	}

	if stoploss.TrailDistance == nil {
		floor, trailDistance, err := stoploss.forecastGeometry(mark, nil)

		if err != nil {
			cancel()
			return nil, err
		}

		stoploss.Floor = floor
		stoploss.TrailDistance = trailDistance
		stoploss.RiskDistance = trailDistance
		stoploss.NoiseBand = trailDistance
	}

	stoploss.Mark = mark
	stoploss.Peak = mark

	if err := stoploss.RebindFill(entryPrice, mark, entryAt); err != nil {
		cancel()
		return nil, err
	}

	return stoploss, nil
}

/*
SetRiskPlan updates the lot's risk plan geometry.
*/
func (stoploss *Stoploss) SetRiskPlan(plan RiskPlan) {
	if stoploss == nil || !plan.Present {
		return
	}

	stoploss.Plan = &plan
	stoploss.RiskDistance = plan.RiskDistance
	stoploss.TrailDistance = plan.TrailDistance
	stoploss.ArmBuffer = plan.ArmBuffer
	stoploss.LockBuffer = plan.LockBuffer
	stoploss.MinEdge = plan.MinEdge
	stoploss.NoiseBand = plan.NoiseBand
	stoploss.ConfirmMarks = plan.ConfirmMarks
}

/*
SetHorizon configures or updates the admitted forecast horizon for this lot.
*/
func (stoploss *Stoploss) SetHorizon(horizon int) {
	if stoploss == nil || horizon < 0 {
		return
	}

	stoploss.Horizon = horizon
}

/*
Maturing reports whether the position is still within its admitted forecast horizon.
*/
func (stoploss *Stoploss) Maturing() bool {
	if stoploss == nil {
		return false
	}

	return stoploss.ClockArmed && stoploss.Horizon > 0 && stoploss.Observed < stoploss.Horizon
}

/*
RebindFill updates the lot from its realized entry and current executable
mark, and stamps entryAt as this lot's identity for persistence. A symbol is
re-entered many times over a session; entryAt is what lets a later restart
tell this lot's stored protection apart from an already-closed trade's.
*/
func (stoploss *Stoploss) RebindFill(
	entryPrice *decimal.Decimal,
	mark *decimal.Decimal,
	entryAt time.Time,
) error {
	profitLine, armAt, lockFloor, err := stoploss.entryGeometry(entryPrice, mark)

	if err != nil {
		return err
	}

	stoploss.Status = ARMED
	stoploss.Mark = mark
	stoploss.Peak = mark
	stoploss.ProfitLine = profitLine
	stoploss.ArmAt = armAt
	stoploss.LockFloor = lockFloor
	stoploss.EntryAt = &entryAt
	stoploss.Locked = false
	stoploss.ProfitLatched = false
	stoploss.DistinctNonPeakMarks = 0
	stoploss.LastStagnationMark = nil
	stoploss.PositiveMoveCount = 0
	stoploss.PositiveMoveMean = 0
	stoploss.PositiveMoveM2 = 0
	stoploss.SurgeArmed = false
	stoploss.LastMove = nil
	stoploss.SurgeMove = nil
	stoploss.MomentumFloor = nil
	stoploss.TriggerReason = ""
	stoploss.TriggerMark = nil

	return nil
}

/*
Update applies the next executable mark without ever lowering the floor, then
advances the forecast-horizon clock. It is the ticker/mark cadence path: each
call counts one admitted observation against the horizon.
*/
func (stoploss *Stoploss) Update(mark *decimal.Decimal) {
	if stoploss == nil || mark == nil || mark.Sign() <= 0 {
		return
	}

	stoploss.observeMark(mark)

	if stoploss.ClockArmed && stoploss.Status == ARMED {
		stoploss.Observed++
	}
}

/*
ObserveExecutable applies the authoritative executable-liquidation state from
one committed L3 book frame. It is the economic-state path: it updates the
executable mark, realizable peak, and trailing floor, and owns every
execution-regime check immediately. It never advances Observed/Horizon — a
busy book that emits many frames must not consume the forecast horizon merely
because more frames arrived.

The mark is the full-lot liquidation-equivalent price. The surface must be
book-complete and fully executable; otherwise the position cannot truthfully
price its complete SellableQty and the protection path claims an
execution-regime invalidation. A locked/protected position whose floor loses
quantity coverage (FloorCoverageQty < SellableQty) is likewise invalidated
immediately: the floor has already become economically unrealizable for the
complete lot even if BestBid still prints above it.
*/
func (stoploss *Stoploss) ObserveExecutable(surface *ExecutionSurface) {
	if stoploss == nil || surface == nil || stoploss.Status == TRIGGERED {
		return
	}

	if !surface.BookComplete {
		// A valid book has never been observed: clean initial bootstrap — the
		// position stays armed and waits for the first coherent frame rather
		// than fabricating a mark or falling back to ticker.
		if !stoploss.BookObserved {
			return
		}

		// A valid book was previously observed and has now become unusable:
		// feed-integrity failure surfaces immediate execution risk.
		stoploss.triggerRegimeInvalidated(surface)
		return
	}

	stoploss.BookObserved = true

	if !surface.FullyExecutable || surface.ExecutableVWAP == nil ||
		surface.ExecutableVWAP.Sign() <= 0 {
		stoploss.triggerRegimeInvalidated(surface)
		return
	}

	if stoploss.Locked && surface.FloorCoverageQty != nil &&
		surface.SellableQty != nil &&
		surface.FloorCoverageQty.Cmp(surface.SellableQty) < 0 {
		stoploss.triggerRegimeInvalidated(surface)
		return
	}

	stoploss.observeMark(surface.ExecutableVWAP)
}

/*
triggerRegimeInvalidated claims the protective exit via the execution-regime
boundary. The executable mark, when available, is retained as the trigger mark;
when the surface cannot price the position at all, the last known executable
mark is retained so the audit record still shows what the lot was worth before
the book became unusable.
*/
func (stoploss *Stoploss) triggerRegimeInvalidated(surface *ExecutionSurface) {
	stoploss.Status = TRIGGERED
	stoploss.TriggerReason = TriggerRegimeInvalidated

	if surface.ExecutableVWAP != nil && surface.ExecutableVWAP.Sign() > 0 {
		stoploss.TriggerMark = decimal.NewFromInt64(0).Add(surface.ExecutableVWAP)
		return
	}

	stoploss.TriggerMark = scaled(stoploss.Mark)
}

/*
observeMark applies one executable mark's full economic evaluation — floor
trigger, peak reach, trailing rachet, profit lock, stagnation, and surge
momentum — without touching the forecast-horizon clock. Update and
ObserveExecutable both delegate here so the mark semantics are identical; only
the caller decides whether the observation counts against the horizon.
*/
func (stoploss *Stoploss) observeMark(mark *decimal.Decimal) {
	if stoploss == nil || mark == nil || mark.Sign() <= 0 {
		return
	}

	previousMark := scaled(stoploss.Mark)
	stoploss.Mark = mark

	if stoploss.Status == TRIGGERED {
		return
	}

	if stoploss.Locked && stoploss.LockFloor != nil &&
		stoploss.LockFloor.Cmp(stoploss.Floor) > 0 {
		stoploss.Floor = stoploss.LockFloor
	}

	// The floor is a boundary, not a debounce hint. The first executable mark
	// at or through it owns the exit immediately.
	if stoploss.Floor != nil && mark.Cmp(stoploss.Floor) <= 0 {
		stoploss.triggerFloor(mark)
		return
	}

	move := stoploss.markMove(previousMark, mark)
	stoploss.LastMove = move

	raisedPeak := stoploss.Peak == nil || mark.Cmp(stoploss.Peak) > 0

	if raisedPeak {
		stoploss.Peak = mark
		stoploss.DistinctNonPeakMarks = 0
		stoploss.LastStagnationMark = nil
	}

	if stoploss.SurgeArmed && stoploss.triggerMomentumExit(mark) {
		return
	}

	// The step is folded before the trailing candidate is placed, so the peak
	// that produced a run-scale step is trailed by a run-scale distance rather
	// than by the entry-time distance computed before it was observed.
	if raisedPeak && move != nil && move.Sign() > 0 {
		stoploss.observePeakMomentum(move, mark)
	}

	candidate := stoploss.trailingCandidate(mark, raisedPeak)

	// Protection begins at the first peak whose own trailing floor can sit
	// above break-even. ArmAt remains a conservative fallback, but no longer
	// delays a valid profit floor merely because the peak rose faster than the
	// static entry geometry expected.
	if !stoploss.Locked && candidate != nil && stoploss.ProfitLine != nil &&
		candidate.Cmp(stoploss.ProfitLine) > 0 {
		stoploss.Locked = true

		if candidate.Cmp(stoploss.Floor) > 0 {
			stoploss.Floor = candidate
		}
	} else if !stoploss.Locked && stoploss.ArmAt != nil &&
		mark.Cmp(stoploss.ArmAt) >= 0 {
		stoploss.Locked = true

		if stoploss.LockFloor != nil && stoploss.LockFloor.Cmp(stoploss.Floor) > 0 {
			stoploss.Floor = stoploss.LockFloor
		}
	}

	if stoploss.Locked && candidate != nil && candidate.Cmp(stoploss.Floor) > 0 {
		stoploss.Floor = candidate
	}

	profitThreshold := stoploss.ProfitLine

	if profitThreshold != nil {
		if stoploss.MinEdge != nil && stoploss.MinEdge.Sign() > 0 {
			profitThreshold = profitThreshold.Add(stoploss.MinEdge)
		}

		if mark.Cmp(profitThreshold) >= 0 {
			stoploss.ProfitLatched = true
		}
	}

	if stoploss.ProfitLatched && !raisedPeak && stoploss.ProfitLine != nil &&
		mark.Cmp(stoploss.ProfitLine) > 0 && !stoploss.isParabolicRun() {
		if stoploss.LastStagnationMark == nil || mark.Cmp(stoploss.LastStagnationMark) != 0 {
			stoploss.DistinctNonPeakMarks++
			stoploss.LastStagnationMark = mark
		}

		confirmMarks := stoploss.ConfirmMarks

		if confirmMarks < 1 {
			confirmMarks = 3
		}

		giveback := scaled(stoploss.Peak).Sub(scaled(mark))

		if stoploss.DistinctNonPeakMarks >= confirmMarks &&
			giveback.Cmp(stoploss.stagnationTolerance()) >= 0 {
			stoploss.Status = TRIGGERED
			stoploss.TriggerReason = TriggerProfitStagnation
			stoploss.TriggerMark = mark
			return
		}
	}
}

func (stoploss *Stoploss) triggerFloor(mark *decimal.Decimal) {
	stoploss.Status = TRIGGERED
	stoploss.TriggerMark = mark

	if stoploss.Locked {
		stoploss.TriggerReason = TriggerProtectedFloor
		return
	}

	stoploss.TriggerReason = TriggerHardFloor
}

func (stoploss *Stoploss) markMove(
	previous *decimal.Decimal,
	mark *decimal.Decimal,
) *decimal.Decimal {
	if previous == nil || previous.Sign() <= 0 || mark == nil {
		return nil
	}

	return scaled(mark).Sub(previous)
}

/*
isParabolicRun reports whether the lot has expanded into a multi-hour or macro
runner (Phase 3). Once in this regime, micro-tick stagnation checks are suppressed
in favor of macro volatility trailing off Peak.
*/
func (stoploss *Stoploss) isParabolicRun() bool {
	if stoploss == nil || stoploss.ProfitLine == nil || stoploss.Peak == nil ||
		stoploss.ProfitLine.Sign() <= 0 {
		return false
	}

	peakFloat := stoploss.Peak.Float64()
	profitFloat := stoploss.ProfitLine.Float64()

	if profitFloat <= 0 {
		return false
	}

	return (peakFloat-profitFloat)/profitFloat >= 0.15
}

/*
trailingCandidate places the floor one giveback tolerance below a new peak. The
tolerance is the wider of the plan's entry distance and the run's own learned
unusual-step boundary, so a floor set while the lot is deep in profit does not
sit an entry-scale distance under a price that now moves in run-scale steps.
*/
func (stoploss *Stoploss) trailingCandidate(
	mark *decimal.Decimal,
	raisedPeak bool,
) *decimal.Decimal {
	if !raisedPeak || mark == nil || stoploss.TrailDistance == nil ||
		stoploss.TrailDistance.Sign() <= 0 {
		return nil
	}

	distance := scaled(stoploss.TrailDistance)

	if learned := stoploss.learnedMoveBoundary(); learned > 0 {
		candidate := decimal.NewFromFloat64(learned)

		if candidate.Cmp(distance) > 0 {
			distance = candidate
		}
	}

	if stoploss.isParabolicRun() {
		markFloat := mark.Float64()
		macroGiveback := decimal.NewFromFloat64(markFloat * 0.10)

		if macroGiveback.Cmp(distance) > 0 {
			distance = macroGiveback
		}
	}

	return floorToTick(
		scaled(mark).Sub(distance),
		stoploss.TickSize,
	)
}

/*
triggerMomentumExit reports whether a surge-armed lot has unwound beyond the
central band of its learned step distribution, and records the exhaustion
trigger when it has. The line is recomputed from live statistics on every mark,
so a burst that keeps extending also keeps raising its own protection.
*/
func (stoploss *Stoploss) triggerMomentumExit(mark *decimal.Decimal) bool {
	stoploss.MomentumFloor = stoploss.armedTrailDistance()

	if stoploss.MomentumFloor == nil || stoploss.MomentumFloor.Sign() <= 0 ||
		stoploss.Peak == nil {
		return false
	}

	momentumLine := floorToTick(
		scaled(stoploss.Peak).Sub(stoploss.MomentumFloor),
		stoploss.TickSize,
	)

	if momentumLine == nil || mark.Cmp(momentumLine) > 0 {
		return false
	}

	stoploss.Status = TRIGGERED
	stoploss.TriggerReason = TriggerPumpMomentumLost
	stoploss.TriggerMark = mark

	return true
}

/*
stagnationTolerance is the giveback below peak that ordinary run dynamics can
still explain: the central band of the learned positive-step distribution,
floored by one execution-noise band. A giveback inside the band is breathing; a
confirmed drift beyond it — several distinct non-peak marks — is a thesis that
has stopped paying for its room.
*/
func (stoploss *Stoploss) stagnationTolerance() *decimal.Decimal {
	tolerance := scaled(stoploss.NoiseBand)

	if tolerance == nil || tolerance.Sign() <= 0 {
		tolerance = scaled(stoploss.TickSize)
	}

	if central := stoploss.centralMoveBoundary(); central > 0 {
		candidate := decimal.NewFromFloat64(central)

		if candidate.Cmp(tolerance) > 0 {
			return candidate
		}
	}

	return tolerance
}

/*
armedTrailDistance is the giveback a surge-armed lot tolerates below its peak
before the burst is treated as unwound: the central band of the learned positive
step distribution, floored by one execution-noise band.
*/
func (stoploss *Stoploss) armedTrailDistance() *decimal.Decimal {
	distance := scaled(stoploss.NoiseBand)

	if distance == nil || distance.Sign() <= 0 {
		distance = scaled(stoploss.TickSize)
	}

	if distance == nil || distance.Sign() <= 0 {
		return nil
	}

	if central := stoploss.centralMoveBoundary(); central > 0 {
		candidate := decimal.NewFromFloat64(central)

		if candidate.Cmp(distance) > 0 {
			return candidate
		}
	}

	return distance
}

/*
learnedMoveBoundary is the statistically unusual positive-step boundary of the
run so far: twice the single observed step before dispersion exists, and the
mean-plus-three-sigma boundary of the learned distribution after that. Zero
means no positive peak step has been observed yet.
*/
func (stoploss *Stoploss) learnedMoveBoundary() float64 {
	if stoploss.PositiveMoveCount == 1 {
		return 2 * stoploss.PositiveMoveMean
	}

	if stoploss.PositiveMoveCount > 1 {
		variance := stoploss.PositiveMoveM2 / float64(stoploss.PositiveMoveCount-1)
		return stoploss.PositiveMoveMean + 3*math.Sqrt(math.Max(0, variance))
	}

	return 0
}

/*
centralMoveBoundary is the mean-plus-one-sigma boundary of the same
distribution: the ordinary scale of the run rather than its tail.
*/
func (stoploss *Stoploss) centralMoveBoundary() float64 {
	if stoploss.PositiveMoveCount < 1 {
		return 0
	}

	variance := 0.0

	if stoploss.PositiveMoveCount > 1 {
		variance = stoploss.PositiveMoveM2 / float64(stoploss.PositiveMoveCount-1)
	}

	return stoploss.PositiveMoveMean + math.Sqrt(math.Max(0, variance))
}

/*
observePeakMomentum learns the ordinary positive-step scale and arms the tighter
surge regime only when a new peak accelerates far beyond it. The arming
threshold is computed before the current step is folded in, so one exceptional
move cannot be made to demand another equally exceptional move. The armed line
itself is derived after the fold, from the distribution the burst now belongs
to.
*/
func (stoploss *Stoploss) observePeakMomentum(
	move *decimal.Decimal,
	mark *decimal.Decimal,
) {
	if move == nil || move.Sign() <= 0 {
		return
	}

	moveValue := move.Float64()
	threshold := stoploss.unusualMoveThreshold()
	profitable := stoploss.Locked || stoploss.ProfitLatched ||
		(stoploss.ProfitLine != nil && mark.Cmp(stoploss.ProfitLine) > 0)

	if profitable && threshold > 0 && moveValue >= threshold {
		stoploss.SurgeArmed = true
		stoploss.SurgeMove = scaled(move)
	}

	stoploss.PositiveMoveCount++
	delta := moveValue - stoploss.PositiveMoveMean
	stoploss.PositiveMoveMean += delta / float64(stoploss.PositiveMoveCount)
	stoploss.PositiveMoveM2 += delta * (moveValue - stoploss.PositiveMoveMean)

	if stoploss.SurgeArmed {
		stoploss.MomentumFloor = stoploss.armedTrailDistance()
	}
}

func (stoploss *Stoploss) unusualMoveThreshold() float64 {
	threshold := 0.0

	if stoploss.TrailDistance != nil {
		threshold = math.Max(threshold, 2*stoploss.TrailDistance.Float64())
	}

	if stoploss.NoiseBand != nil {
		threshold = math.Max(threshold, 4*stoploss.NoiseBand.Float64())
	}

	if stoploss.TickSize != nil {
		threshold = math.Max(threshold, 4*stoploss.TickSize.Float64())
	}

	return math.Max(threshold, stoploss.learnedMoveBoundary())
}

/*
ArmClock starts the lot's own forecast-horizon clock after the entry fill.
Marks observed before the fill do not count against the admitted path.
*/
func (stoploss *Stoploss) ArmClock() {
	if stoploss == nil || stoploss.ClockArmed {
		return
	}

	stoploss.ClockArmed = true
	stoploss.Observed = 0
}

/*
Reconsider releases a still-red lot once the transition horizon that justified
its entry has elapsed. The retained parameters are ignored for source compatibility:
future return economics no longer receive a second, hidden vote over the observed
position. Missing horizon evidence keeps the lot; profitable and locked positions
remain governed by Update's floor, trail, and momentum paths.
*/
func (stoploss *Stoploss) Reconsider(_ float64, _ float64) {
	if stoploss == nil || stoploss.Status == TRIGGERED || stoploss.Locked {
		return
	}

	if stoploss.ProfitLine != nil && stoploss.Mark != nil &&
		stoploss.Mark.Cmp(stoploss.ProfitLine) >= 0 {
		return
	}

	if !stoploss.ClockArmed || stoploss.Horizon < 1 ||
		stoploss.Observed < stoploss.Horizon {
		return
	}

	stoploss.Status = TRIGGERED
	stoploss.TriggerReason = TriggerHorizonExpired
	stoploss.TriggerMark = stoploss.Mark
}

/*
TriggerManualOverride records an operator-requested exit through the same
regulator state boundary used by automatic exits. It never fabricates a price:
the latest executable mark already owned by the stop is retained as the
trigger mark.
*/
func (stoploss *Stoploss) TriggerManualOverride() error {
	if stoploss == nil {
		return errnie.Err(
			errnie.Validation,
			"stoploss: manual override requires an active regulator",
			nil,
		)
	}

	if stoploss.Status == TRIGGERED {
		return nil
	}

	if stoploss.Status != ARMED {
		return errnie.Err(
			errnie.NotAcceptable,
			"stoploss: only an armed position can be manually exited",
			nil,
		)
	}

	stoploss.Status = TRIGGERED
	stoploss.TriggerReason = TriggerManualOverride
	stoploss.TriggerMark = scaled(stoploss.Mark)
	return nil
}

/*
MarshalState encodes the live values needed to continue regulating the lot.
*/
func (stoploss *Stoploss) MarshalState() ([]byte, error) {
	if stoploss == nil {
		return nil, fmt.Errorf("stoploss: state required")
	}

	return json.Marshal(stoploss)
}

/*
RestoreStoploss resumes a regulator from its stored live state.
*/
func RestoreStoploss(ctx context.Context, encoded []byte) (*Stoploss, error) {
	stoploss := &Stoploss{}

	if err := json.Unmarshal(encoded, stoploss); err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Internal,
			"stoploss: decode state: %w",
			err,
		))
	}

	if stoploss.Symbol == "" || stoploss.TickSize == nil || stoploss.TickSize.Sign() <= 0 ||
		stoploss.TrailDistance == nil || stoploss.TrailDistance.Sign() <= 0 ||
		stoploss.Floor == nil || stoploss.Floor.Sign() <= 0 ||
		stoploss.Mark == nil || stoploss.Mark.Sign() <= 0 ||
		stoploss.Peak == nil || stoploss.Peak.Sign() <= 0 ||
		stoploss.ProfitLine == nil || stoploss.ProfitLine.Sign() <= 0 ||
		stoploss.ArmAt == nil || stoploss.ArmAt.Sign() <= 0 ||
		stoploss.LockFloor == nil || stoploss.LockFloor.Sign() <= 0 ||
		stoploss.EntryAt == nil || stoploss.EntryAt.IsZero() {
		return nil, fmt.Errorf("stoploss: complete stored state required")
	}

	if stoploss.Status != ARMED && stoploss.Status != TRIGGERED && stoploss.Status != ERROR {
		return nil, fmt.Errorf("stoploss: invalid stored status %s", stoploss.Status)
	}

	if stoploss.Floor.Cmp(stoploss.Peak) >= 0 {
		return nil, fmt.Errorf("stoploss: stored floor must remain below peak")
	}

	if stoploss.Horizon < 0 || stoploss.Observed < 0 || stoploss.PositiveMoveCount < 0 ||
		math.IsNaN(stoploss.PositiveMoveMean) || math.IsInf(stoploss.PositiveMoveMean, 0) ||
		math.IsNaN(stoploss.PositiveMoveM2) || math.IsInf(stoploss.PositiveMoveM2, 0) ||
		stoploss.PositiveMoveM2 < 0 {
		return nil, fmt.Errorf("stoploss: stored horizon or momentum state is invalid")
	}

	if stoploss.SurgeArmed && (stoploss.SurgeMove == nil || stoploss.SurgeMove.Sign() <= 0 ||
		stoploss.MomentumFloor == nil || stoploss.MomentumFloor.Sign() <= 0) {
		return nil, fmt.Errorf("stoploss: armed surge requires positive momentum geometry")
	}

	if stoploss.ConfirmMarks < 1 {
		stoploss.ConfirmMarks = 3
	}

	ctx, cancel := context.WithCancel(ctx)
	stoploss.ctx = ctx
	stoploss.cancel = cancel

	return stoploss, nil
}

func (stoploss *Stoploss) forecastGeometry(
	mark *decimal.Decimal,
	forwardCurve []float64,
) (*decimal.Decimal, *decimal.Decimal, error) {
	if mark == nil || mark.Sign() <= 0 {
		return nil, nil, fmt.Errorf("stoploss: positive mark required")
	}

	if stoploss.forecast == nil {
		return nil, nil, fmt.Errorf("stoploss: forecast required")
	}

	tick := scaled(stoploss.TickSize)

	if tick == nil || tick.Sign() <= 0 {
		return nil, nil, fmt.Errorf(
			"stoploss: positive tick size required",
		)
	}

	if !stoploss.forecast.Ready {
		return nil, nil, fmt.Errorf("stoploss: forecast distribution required")
	}

	if len(forwardCurve) == 0 {
		currentMark := scaled(mark)
		floor := floorToTick(currentMark.Sub(tick), tick)

		if floor == nil || floor.Sign() <= 0 || floor.Cmp(currentMark) >= 0 {
			return nil, nil, fmt.Errorf(
				"stoploss: cost lattice does not imply a positive floor",
			)
		}

		return floor, tick, nil
	}

	minimumPathReturn := 0.0
	cumulativeReturn := 0.0

	for _, predictedReturn := range forwardCurve {
		cumulativeReturn += predictedReturn

		if cumulativeReturn < minimumPathReturn {
			minimumPathReturn = cumulativeReturn
		}
	}

	drawdown := -math.Expm1(minimumPathReturn)

	if drawdown < 0 {
		drawdown = 0
	}

	currentMark := scaled(mark)
	one := decimal.NewFromInt64(1)
	survival := one.Sub(decimal.NewFromFloat64(drawdown))
	floor := floorToTick(currentMark.Mul(survival), tick)

	if floor != nil && floor.Cmp(currentMark) >= 0 {
		floor = floorToTick(currentMark.Sub(tick), tick)
	}

	if floor == nil || floor.Sign() <= 0 {
		return nil, nil, fmt.Errorf(
			"stoploss: forecast does not imply a positive floor",
		)
	}

	if floor.Cmp(currentMark) >= 0 {
		return nil, nil, fmt.Errorf(
			"stoploss: forecast floor must remain below executable mark",
		)
	}

	trailDistance := ceilToTick(currentMark.Sub(floor), tick)

	if trailDistance == nil || trailDistance.Cmp(tick) < 0 {
		trailDistance = tick
	}

	return floor, trailDistance, nil
}

func (stoploss *Stoploss) entryGeometry(
	entryPrice *decimal.Decimal,
	mark *decimal.Decimal,
) (*decimal.Decimal, *decimal.Decimal, *decimal.Decimal, error) {
	if entryPrice == nil || entryPrice.Sign() <= 0 {
		return nil, nil, nil, fmt.Errorf(
			"stoploss: positive entry price required",
		)
	}

	if mark == nil || mark.Sign() <= 0 {
		return nil, nil, nil, fmt.Errorf(
			"stoploss: positive executable mark required",
		)
	}

	tick := scaled(stoploss.TickSize)

	if tick == nil || tick.Sign() <= 0 {
		return nil, nil, nil, fmt.Errorf("stoploss: tick size required")
	}

	entry := scaled(entryPrice)
	one := decimal.NewFromInt64(1)
	entryRate := scaled(stoploss.EntryFeeRate)
	exitRate := scaled(stoploss.ExitFeeRate)

	if entryRate == nil || entryRate.Sign() < 0 ||
		exitRate == nil || exitRate.Sign() < 0 || exitRate.Cmp(one) >= 0 {
		return nil, nil, nil, fmt.Errorf(
			"stoploss: valid fee rates required",
		)
	}

	cost := entry.Add(entry.Mul(entryRate))
	breakEven := cost.Div(one.Sub(exitRate))
	profitLine := ceilToTick(breakEven, tick)
	currentMark := scaled(mark)

	var armAt, lockFloor *decimal.Decimal

	if stoploss.ArmBuffer != nil && stoploss.ArmBuffer.Sign() > 0 &&
		stoploss.LockBuffer != nil && stoploss.LockBuffer.Sign() > 0 {
		armAt = ceilToTick(profitLine.Add(stoploss.ArmBuffer), tick)
		lockFloor = floorToTick(profitLine.Add(stoploss.LockBuffer), tick)
	} else {
		trailDistance := stoploss.TrailDistance

		if trailDistance == nil || trailDistance.Sign() <= 0 {
			trailDistance = tick
		}

		armAt = ceilToTick(
			profitLine.Add(trailDistance).Add(tick),
			tick,
		)
		lockFloor = floorToTick(
			armAt.Sub(trailDistance),
			tick,
		)
	}

	if lockFloor == nil || lockFloor.Cmp(profitLine) <= 0 {
		lockFloor = ceilToTick(profitLine.Add(tick), tick)
	}

	if armAt.Cmp(lockFloor) <= 0 {
		armAt = ceilToTick(lockFloor.Add(tick), tick)
	}

	var executionFloor *decimal.Decimal

	if stoploss.Plan != nil && stoploss.Plan.Present &&
		stoploss.RiskDistance != nil && stoploss.RiskDistance.Sign() > 0 {
		executionFloor = floorToTick(currentMark.Sub(stoploss.RiskDistance), tick)
	} else {
		executionDistance := ceilToTick(profitLine.Sub(currentMark), tick)
		riskDistance := largest(stoploss.RiskDistance, executionDistance)
		executionFloor = floorToTick(currentMark.Sub(riskDistance), tick)
		stoploss.RiskDistance = riskDistance
	}

	if executionFloor == nil || executionFloor.Sign() <= 0 {
		return nil, nil, nil, fmt.Errorf(
			"stoploss: execution cost does not imply a positive floor",
		)
	}

	if stoploss.Floor == nil || executionFloor.Cmp(stoploss.Floor) < 0 {
		stoploss.Floor = executionFloor
	}

	return profitLine, armAt, lockFloor, nil
}

/*
Close cancels the regulator context.
*/
func (stoploss *Stoploss) Close() (err error) {
	if stoploss.cancel != nil {
		stoploss.cancel()
	}

	return errnie.Error(err)
}
