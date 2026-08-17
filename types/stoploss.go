package types

import (
	"context"
	"encoding/json"
	"fmt"
	"math"

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
	TriggerContinuationEV    = "continuation_ev_negative"
	TriggerRegimeInvalidated = "execution_regime_invalidated"
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
	ctx                  context.Context
	cancel               context.CancelFunc
	forecast             *learning.RLSOutput
	tickSize             *decimal.Decimal
	entryFeeRate         *decimal.Decimal
	exitFeeRate          *decimal.Decimal
	riskDistance         *decimal.Decimal
	trailDistance        *decimal.Decimal
	armBuffer            *decimal.Decimal
	lockBuffer           *decimal.Decimal
	minEdge              *decimal.Decimal
	noiseBand            *decimal.Decimal
	confirmMarks         int
	distinctNonPeakMarks int
	lastStagnationMark   *decimal.Decimal
	profitLatched        bool
	positiveMoveCount    int
	positiveMoveMean     float64
	positiveMoveM2       float64
	horizon              int
	observed             int
	clockArmed           bool
	Status               Status           `json:"status"`
	Symbol               string           `json:"symbol"`
	Floor                *decimal.Decimal `json:"floor"`
	Mark                 *decimal.Decimal `json:"mark"`
	Peak                 *decimal.Decimal `json:"peak"`
	ProfitLine           *decimal.Decimal `json:"profit_line"`
	ArmAt                *decimal.Decimal `json:"arm_at"`
	LockFloor            *decimal.Decimal `json:"lock_floor"`
	Locked               bool             `json:"locked"`
	TriggerReason        string           `json:"trigger_reason,omitempty"`
	TriggerMark          *decimal.Decimal `json:"trigger_mark,omitempty"`
	SurgeArmed           bool             `json:"surge_armed"`
	LastMove             *decimal.Decimal `json:"last_move,omitempty"`
	SurgeMove            *decimal.Decimal `json:"surge_move,omitempty"`
	MomentumFloor        *decimal.Decimal `json:"momentum_floor,omitempty"`
	Plan                 *RiskPlan        `json:"plan,omitempty"`
}

type stoplossState struct {
	Status               Status           `json:"status"`
	Symbol               string           `json:"symbol"`
	TickSize             *decimal.Decimal `json:"tick_size"`
	EntryFeeRate         *decimal.Decimal `json:"entry_fee_rate,omitempty"`
	ExitFeeRate          *decimal.Decimal `json:"exit_fee_rate,omitempty"`
	RiskDistance         *decimal.Decimal `json:"risk_distance,omitempty"`
	TrailDistance        *decimal.Decimal `json:"trail_distance"`
	ArmBuffer            *decimal.Decimal `json:"arm_buffer,omitempty"`
	LockBuffer           *decimal.Decimal `json:"lock_buffer,omitempty"`
	MinEdge              *decimal.Decimal `json:"min_edge,omitempty"`
	NoiseBand            *decimal.Decimal `json:"noise_band,omitempty"`
	ConfirmMarks         int              `json:"confirm_marks,omitempty"`
	DistinctNonPeakMarks int              `json:"distinct_non_peak_marks,omitempty"`
	ProfitLatched        bool             `json:"profit_latched,omitempty"`
	PositiveMoveCount    int              `json:"positive_move_count,omitempty"`
	PositiveMoveMean     float64          `json:"positive_move_mean,omitempty"`
	PositiveMoveM2       float64          `json:"positive_move_m2,omitempty"`
	Horizon              int              `json:"horizon"`
	Observed             int              `json:"observed"`
	ClockArmed           bool             `json:"clock_armed"`
	Floor                *decimal.Decimal `json:"floor"`
	Mark                 *decimal.Decimal `json:"mark"`
	Peak                 *decimal.Decimal `json:"peak"`
	ProfitLine           *decimal.Decimal `json:"profit_line"`
	ArmAt                *decimal.Decimal `json:"arm_at"`
	LockFloor            *decimal.Decimal `json:"lock_floor"`
	Locked               bool             `json:"locked"`
	TriggerReason        string           `json:"trigger_reason,omitempty"`
	TriggerMark          *decimal.Decimal `json:"trigger_mark,omitempty"`
	SurgeArmed           bool             `json:"surge_armed,omitempty"`
	LastMove             *decimal.Decimal `json:"last_move,omitempty"`
	SurgeMove            *decimal.Decimal `json:"surge_move,omitempty"`
	MomentumFloor        *decimal.Decimal `json:"momentum_floor,omitempty"`
	Plan                 *RiskPlan        `json:"plan,omitempty"`
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
) (*Stoploss, error) {
	if symbol == "" {
		return nil, fmt.Errorf("stoploss: symbol required")
	}

	ctx, cancel := context.WithCancel(ctx)
	stoploss := &Stoploss{
		ctx:          ctx,
		cancel:       cancel,
		forecast:     forecast,
		tickSize:     tickSize,
		entryFeeRate: entryFeeRate,
		exitFeeRate:  exitFeeRate,
		Status:       ARMED,
		Symbol:       symbol,
		horizon:      len(forwardCurve),
		confirmMarks: 3,
		minEdge:      tickSize,
	}

	floor, trailDistance, err := stoploss.forecastGeometry(mark, forwardCurve)

	if err != nil {
		cancel()
		return nil, err
	}

	stoploss.Mark = mark
	stoploss.Peak = mark
	stoploss.Floor = floor
	stoploss.trailDistance = trailDistance
	stoploss.riskDistance = trailDistance
	stoploss.noiseBand = trailDistance

	if err := stoploss.RebindFill(entryPrice, mark); err != nil {
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
) (*Stoploss, error) {
	if symbol == "" {
		return nil, fmt.Errorf("stoploss: symbol required")
	}

	ctx, cancel := context.WithCancel(ctx)
	stoploss := &Stoploss{
		ctx:          ctx,
		cancel:       cancel,
		forecast:     forecast,
		tickSize:     tickSize,
		entryFeeRate: entryFeeRate,
		exitFeeRate:  exitFeeRate,
		Status:       ARMED,
		Symbol:       symbol,
		horizon:      horizon,
		confirmMarks: 3,
		minEdge:      tickSize,
	}

	if plan != nil && plan.Present {
		stoploss.SetRiskPlan(*plan)
	}

	if stoploss.trailDistance == nil {
		floor, trailDistance, err := stoploss.forecastGeometry(mark, nil)

		if err != nil {
			cancel()
			return nil, err
		}

		stoploss.Floor = floor
		stoploss.trailDistance = trailDistance
		stoploss.riskDistance = trailDistance
		stoploss.noiseBand = trailDistance
	}

	stoploss.Mark = mark
	stoploss.Peak = mark

	if err := stoploss.RebindFill(entryPrice, mark); err != nil {
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
	stoploss.riskDistance = plan.RiskDistance
	stoploss.trailDistance = plan.TrailDistance
	stoploss.armBuffer = plan.ArmBuffer
	stoploss.lockBuffer = plan.LockBuffer
	stoploss.minEdge = plan.MinEdge
	stoploss.noiseBand = plan.NoiseBand
	stoploss.confirmMarks = plan.ConfirmMarks
}

/*
SetHorizon configures or updates the admitted forecast horizon for this lot.
*/
func (stoploss *Stoploss) SetHorizon(horizon int) {
	if stoploss == nil || horizon < 0 {
		return
	}

	stoploss.horizon = horizon
}

/*
RebindFill updates the lot from its realized entry and current executable mark.
*/
func (stoploss *Stoploss) RebindFill(
	entryPrice *decimal.Decimal,
	mark *decimal.Decimal,
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
	stoploss.Locked = false
	stoploss.profitLatched = false
	stoploss.distinctNonPeakMarks = 0
	stoploss.lastStagnationMark = nil
	stoploss.positiveMoveCount = 0
	stoploss.positiveMoveMean = 0
	stoploss.positiveMoveM2 = 0
	stoploss.SurgeArmed = false
	stoploss.LastMove = nil
	stoploss.SurgeMove = nil
	stoploss.MomentumFloor = nil
	stoploss.TriggerReason = ""
	stoploss.TriggerMark = nil

	return nil
}

/*
Update applies the next executable mark without ever lowering the floor.
*/
func (stoploss *Stoploss) Update(mark *decimal.Decimal) {
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
		stoploss.distinctNonPeakMarks = 0
		stoploss.lastStagnationMark = nil
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
		if stoploss.minEdge != nil && stoploss.minEdge.Sign() > 0 {
			profitThreshold = profitThreshold.Add(stoploss.minEdge)
		}

		if mark.Cmp(profitThreshold) >= 0 {
			stoploss.profitLatched = true
		}
	}

	if stoploss.profitLatched && !raisedPeak && stoploss.ProfitLine != nil &&
		mark.Cmp(stoploss.ProfitLine) > 0 {
		if stoploss.lastStagnationMark == nil || mark.Cmp(stoploss.lastStagnationMark) != 0 {
			stoploss.distinctNonPeakMarks++
			stoploss.lastStagnationMark = mark
		}

		confirmMarks := stoploss.confirmMarks

		if confirmMarks < 1 {
			confirmMarks = 3
		}

		giveback := scaled(stoploss.Peak).Sub(scaled(mark))

		if stoploss.distinctNonPeakMarks >= confirmMarks &&
			giveback.Cmp(stoploss.stagnationTolerance()) >= 0 {
			stoploss.Status = TRIGGERED
			stoploss.TriggerReason = TriggerProfitStagnation
			stoploss.TriggerMark = mark
			return
		}
	}

	if stoploss.clockArmed && stoploss.Status == ARMED {
		stoploss.observed++
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
trailingCandidate places the floor one giveback tolerance below a new peak. The
tolerance is the wider of the plan's entry distance and the run's own learned
unusual-step boundary, so a floor set while the lot is deep in profit does not
sit an entry-scale distance under a price that now moves in run-scale steps.
*/
func (stoploss *Stoploss) trailingCandidate(
	mark *decimal.Decimal,
	raisedPeak bool,
) *decimal.Decimal {
	if !raisedPeak || mark == nil || stoploss.trailDistance == nil ||
		stoploss.trailDistance.Sign() <= 0 {
		return nil
	}

	distance := scaled(stoploss.trailDistance)

	if learned := stoploss.learnedMoveBoundary(); learned > 0 {
		candidate := decimal.NewFromFloat64(learned)

		if candidate.Cmp(distance) > 0 {
			distance = candidate
		}
	}

	return floorToTick(
		scaled(mark).Sub(distance),
		stoploss.tickSize,
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
		stoploss.tickSize,
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
	tolerance := scaled(stoploss.noiseBand)

	if tolerance == nil || tolerance.Sign() <= 0 {
		tolerance = scaled(stoploss.tickSize)
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
	distance := scaled(stoploss.noiseBand)

	if distance == nil || distance.Sign() <= 0 {
		distance = scaled(stoploss.tickSize)
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
	if stoploss.positiveMoveCount == 1 {
		return 2 * stoploss.positiveMoveMean
	}

	if stoploss.positiveMoveCount > 1 {
		variance := stoploss.positiveMoveM2 / float64(stoploss.positiveMoveCount-1)
		return stoploss.positiveMoveMean + 3*math.Sqrt(math.Max(0, variance))
	}

	return 0
}

/*
centralMoveBoundary is the mean-plus-one-sigma boundary of the same
distribution: the ordinary scale of the run rather than its tail.
*/
func (stoploss *Stoploss) centralMoveBoundary() float64 {
	if stoploss.positiveMoveCount < 1 {
		return 0
	}

	variance := 0.0

	if stoploss.positiveMoveCount > 1 {
		variance = stoploss.positiveMoveM2 / float64(stoploss.positiveMoveCount-1)
	}

	return stoploss.positiveMoveMean + math.Sqrt(math.Max(0, variance))
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
	profitable := stoploss.Locked || stoploss.profitLatched ||
		(stoploss.ProfitLine != nil && mark.Cmp(stoploss.ProfitLine) > 0)

	if profitable && threshold > 0 && moveValue >= threshold {
		stoploss.SurgeArmed = true
		stoploss.SurgeMove = scaled(move)
	}

	stoploss.positiveMoveCount++
	delta := moveValue - stoploss.positiveMoveMean
	stoploss.positiveMoveMean += delta / float64(stoploss.positiveMoveCount)
	stoploss.positiveMoveM2 += delta * (moveValue - stoploss.positiveMoveMean)

	if stoploss.SurgeArmed {
		stoploss.MomentumFloor = stoploss.armedTrailDistance()
	}
}

func (stoploss *Stoploss) unusualMoveThreshold() float64 {
	threshold := 0.0

	if stoploss.trailDistance != nil {
		threshold = math.Max(threshold, 2*stoploss.trailDistance.Float64())
	}

	if stoploss.noiseBand != nil {
		threshold = math.Max(threshold, 4*stoploss.noiseBand.Float64())
	}

	if stoploss.tickSize != nil {
		threshold = math.Max(threshold, 4*stoploss.tickSize.Float64())
	}

	return math.Max(threshold, stoploss.learnedMoveBoundary())
}

/*
ArmClock starts the lot's own forecast-horizon clock after the entry fill.
Marks observed before the fill do not count against the admitted path.
*/
func (stoploss *Stoploss) ArmClock() {
	if stoploss == nil || stoploss.clockArmed {
		return
	}

	stoploss.clockArmed = true
	stoploss.observed = 0
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

	if !stoploss.clockArmed || stoploss.horizon < 1 ||
		stoploss.observed < stoploss.horizon {
		return
	}

	stoploss.Status = TRIGGERED
	stoploss.TriggerReason = TriggerHorizonExpired
	stoploss.TriggerMark = stoploss.Mark
}

/*
MarshalState encodes the live values needed to continue regulating the lot.
*/
func (stoploss *Stoploss) MarshalState() ([]byte, error) {
	if stoploss == nil {
		return nil, fmt.Errorf("stoploss: state required")
	}

	return json.Marshal(stoplossState{
		Status:               stoploss.Status,
		Symbol:               stoploss.Symbol,
		TickSize:             stoploss.tickSize,
		EntryFeeRate:         stoploss.entryFeeRate,
		ExitFeeRate:          stoploss.exitFeeRate,
		RiskDistance:         stoploss.riskDistance,
		TrailDistance:        stoploss.trailDistance,
		ArmBuffer:            stoploss.armBuffer,
		LockBuffer:           stoploss.lockBuffer,
		MinEdge:              stoploss.minEdge,
		NoiseBand:            stoploss.noiseBand,
		ConfirmMarks:         stoploss.confirmMarks,
		DistinctNonPeakMarks: stoploss.distinctNonPeakMarks,
		ProfitLatched:        stoploss.profitLatched,
		PositiveMoveCount:    stoploss.positiveMoveCount,
		PositiveMoveMean:     stoploss.positiveMoveMean,
		PositiveMoveM2:       stoploss.positiveMoveM2,
		Horizon:              stoploss.horizon,
		Observed:             stoploss.observed,
		ClockArmed:           stoploss.clockArmed,
		Floor:                stoploss.Floor,
		Mark:                 stoploss.Mark,
		Peak:                 stoploss.Peak,
		ProfitLine:           stoploss.ProfitLine,
		ArmAt:                stoploss.ArmAt,
		LockFloor:            stoploss.LockFloor,
		Locked:               stoploss.Locked,
		TriggerReason:        stoploss.TriggerReason,
		TriggerMark:          stoploss.TriggerMark,
		SurgeArmed:           stoploss.SurgeArmed,
		LastMove:             stoploss.LastMove,
		SurgeMove:            stoploss.SurgeMove,
		MomentumFloor:        stoploss.MomentumFloor,
		Plan:                 stoploss.Plan,
	})
}

/*
RestoreStoploss resumes a regulator from its stored live state.
*/
func RestoreStoploss(ctx context.Context, encoded []byte) (*Stoploss, error) {
	state := stoplossState{}

	if err := json.Unmarshal(encoded, &state); err != nil {
		return nil, fmt.Errorf("stoploss: decode state: %w", err)
	}

	if state.Symbol == "" || state.TickSize == nil || state.TickSize.Sign() <= 0 ||
		state.TrailDistance == nil || state.TrailDistance.Sign() <= 0 ||
		state.Floor == nil || state.Floor.Sign() <= 0 ||
		state.Mark == nil || state.Mark.Sign() <= 0 ||
		state.Peak == nil || state.Peak.Sign() <= 0 ||
		state.ProfitLine == nil || state.ProfitLine.Sign() <= 0 ||
		state.ArmAt == nil || state.ArmAt.Sign() <= 0 ||
		state.LockFloor == nil || state.LockFloor.Sign() <= 0 {
		return nil, fmt.Errorf("stoploss: complete stored state required")
	}

	if state.Status != ARMED && state.Status != TRIGGERED && state.Status != ERROR {
		return nil, fmt.Errorf("stoploss: invalid stored status %s", state.Status)
	}

	if state.Floor.Cmp(state.Peak) >= 0 {
		return nil, fmt.Errorf("stoploss: stored floor must remain below peak")
	}

	if state.Horizon < 0 || state.Observed < 0 || state.PositiveMoveCount < 0 ||
		math.IsNaN(state.PositiveMoveMean) || math.IsInf(state.PositiveMoveMean, 0) ||
		math.IsNaN(state.PositiveMoveM2) || math.IsInf(state.PositiveMoveM2, 0) ||
		state.PositiveMoveM2 < 0 {
		return nil, fmt.Errorf("stoploss: stored horizon or momentum state is invalid")
	}

	if state.SurgeArmed && (state.SurgeMove == nil || state.SurgeMove.Sign() <= 0 ||
		state.MomentumFloor == nil || state.MomentumFloor.Sign() <= 0) {
		return nil, fmt.Errorf("stoploss: armed surge requires positive momentum geometry")
	}

	ctx, cancel := context.WithCancel(ctx)

	confirmMarks := state.ConfirmMarks
	if confirmMarks < 1 {
		confirmMarks = 3
	}

	return &Stoploss{
		ctx:                  ctx,
		cancel:               cancel,
		tickSize:             state.TickSize,
		entryFeeRate:         state.EntryFeeRate,
		exitFeeRate:          state.ExitFeeRate,
		riskDistance:         state.RiskDistance,
		trailDistance:        state.TrailDistance,
		armBuffer:            state.ArmBuffer,
		lockBuffer:           state.LockBuffer,
		minEdge:              state.MinEdge,
		noiseBand:            state.NoiseBand,
		confirmMarks:         confirmMarks,
		distinctNonPeakMarks: state.DistinctNonPeakMarks,
		profitLatched:        state.ProfitLatched,
		positiveMoveCount:    state.PositiveMoveCount,
		positiveMoveMean:     state.PositiveMoveMean,
		positiveMoveM2:       state.PositiveMoveM2,
		horizon:              state.Horizon,
		observed:             state.Observed,
		clockArmed:           state.ClockArmed,
		Status:               state.Status,
		Symbol:               state.Symbol,
		Floor:                state.Floor,
		Mark:                 state.Mark,
		Peak:                 state.Peak,
		ProfitLine:           state.ProfitLine,
		ArmAt:                state.ArmAt,
		LockFloor:            state.LockFloor,
		Locked:               state.Locked,
		TriggerReason:        state.TriggerReason,
		TriggerMark:          state.TriggerMark,
		SurgeArmed:           state.SurgeArmed,
		LastMove:             state.LastMove,
		SurgeMove:            state.SurgeMove,
		MomentumFloor:        state.MomentumFloor,
		Plan:                 state.Plan,
	}, nil
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

	tick := scaled(stoploss.tickSize)

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

	tick := scaled(stoploss.tickSize)

	if tick == nil || tick.Sign() <= 0 {
		return nil, nil, nil, fmt.Errorf("stoploss: tick size required")
	}

	entry := scaled(entryPrice)
	one := decimal.NewFromInt64(1)
	entryRate := scaled(stoploss.entryFeeRate)
	exitRate := scaled(stoploss.exitFeeRate)

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

	if stoploss.armBuffer != nil && stoploss.armBuffer.Sign() > 0 &&
		stoploss.lockBuffer != nil && stoploss.lockBuffer.Sign() > 0 {
		armAt = ceilToTick(profitLine.Add(stoploss.armBuffer), tick)
		lockFloor = floorToTick(profitLine.Add(stoploss.lockBuffer), tick)
	} else {
		trailDistance := stoploss.trailDistance

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
		stoploss.riskDistance != nil && stoploss.riskDistance.Sign() > 0 {
		executionFloor = floorToTick(currentMark.Sub(stoploss.riskDistance), tick)
	} else {
		executionDistance := ceilToTick(profitLine.Sub(currentMark), tick)
		riskDistance := largest(stoploss.riskDistance, executionDistance)
		executionFloor = floorToTick(currentMark.Sub(riskDistance), tick)
		stoploss.riskDistance = riskDistance
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
