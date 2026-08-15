package types

import (
	"context"
	"encoding/json"
	"fmt"
	"math"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/learning"
)

const (
	TriggerHardFloor         = "hard_floor"
	TriggerProtectedFloor    = "protected_floor"
	TriggerProfitStagnation  = "profit_stagnation"
	TriggerTrailingFloor     = "trailing_floor"
	TriggerHorizonExpired    = "horizon_expired"
	TriggerContinuationEV    = "continuation_ev_negative"
	TriggerRegimeInvalidated = "execution_regime_invalidated"
)

/*
Stoploss regulates one open lot across discovery, protected profit, and trailing regimes.

Hard loss floor is bounded by RiskDistance.
Once Mark clears break-even plus MinEdge, profit is latched.
ArmAt sits ArmBuffer above break-even; when reached, the protected floor locks at LockFloor
(LockBuffer above break-even). New peaks ratchet the trailing floor upward by TrailDistance.

A profitable lot triggers profit stagnation only after multiple distinct non-peak observations
give back at least one execution noise band from the peak.
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

	return nil
}

/*
Update applies the next executable mark without ever lowering the floor.
*/
func (stoploss *Stoploss) Update(mark *decimal.Decimal) {
	if stoploss == nil || mark == nil || mark.Sign() <= 0 {
		return
	}

	stoploss.Mark = mark

	if stoploss.Status == TRIGGERED {
		return
	}

	if stoploss.Locked && stoploss.LockFloor != nil &&
		stoploss.LockFloor.Cmp(stoploss.Floor) > 0 {
		stoploss.Floor = stoploss.LockFloor
	}

	if mark.Cmp(stoploss.Floor) < 0 {
		stoploss.Status = TRIGGERED
		stoploss.TriggerMark = mark

		if stoploss.Locked {
			stoploss.TriggerReason = TriggerProtectedFloor
		} else {
			stoploss.TriggerReason = TriggerHardFloor
		}

		return
	}

	raisedPeak := mark.Cmp(stoploss.Peak) > 0

	if raisedPeak {
		stoploss.Peak = mark
		stoploss.distinctNonPeakMarks = 0
		stoploss.lastStagnationMark = nil
	}

	if !stoploss.Locked && mark.Cmp(stoploss.ArmAt) >= 0 {
		stoploss.Locked = true

		if stoploss.LockFloor.Cmp(stoploss.Floor) > 0 {
			stoploss.Floor = stoploss.LockFloor
		}
	}

	if stoploss.Locked && raisedPeak && stoploss.trailDistance != nil {
		candidate := floorToTick(
			scaled(mark).Sub(stoploss.trailDistance),
			stoploss.tickSize,
		)

		if candidate != nil && candidate.Cmp(stoploss.Floor) > 0 {
			stoploss.Floor = candidate
		}
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

		noiseBand := stoploss.noiseBand

		if noiseBand == nil || noiseBand.Sign() <= 0 {
			if stoploss.tickSize != nil {
				noiseBand = stoploss.tickSize
			} else {
				noiseBand = decimal.NewFromInt64(0)
			}
		}

		giveback := scaled(stoploss.Peak).Sub(scaled(mark))

		if stoploss.distinctNonPeakMarks >= confirmMarks && giveback.Cmp(noiseBand) >= 0 {
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
Reconsider releases a still-red lot after its admitted horizon has been
observed, unless the live forecast still clears the regulated utility gate
and is still expected to reach the profit line. Missing evidence keeps the
lot; profit-lock and in-profit stagnation stay on Update.
*/
func (stoploss *Stoploss) Reconsider(
	forecastLogReturn float64,
	threshold float64,
) {
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

	exitRate := 0.0

	if stoploss.exitFeeRate != nil {
		exitRate = stoploss.exitFeeRate.Float64()
	}

	if stayUtility(forecastLogReturn, exitRate) > threshold &&
		stoploss.reachesProfit(forecastLogReturn) {
		return
	}

	stoploss.Status = TRIGGERED
	stoploss.TriggerReason = TriggerHorizonExpired
	stoploss.TriggerMark = stoploss.Mark
}

/*
stayUtility is the remaining arithmetic return after the exit fee still owed.
The entry fee is sunk and is not charged again.
*/
func stayUtility(forecastLogReturn, exitFeeRate float64) float64 {
	return math.Expm1(forecastLogReturn) * (1 - exitFeeRate)
}

func (stoploss *Stoploss) reachesProfit(forecastLogReturn float64) bool {
	if stoploss.Mark == nil || stoploss.ProfitLine == nil ||
		stoploss.Mark.Sign() <= 0 || stoploss.ProfitLine.Sign() <= 0 {
		return false
	}

	return stoploss.Mark.Float64()*math.Exp(forecastLogReturn) >=
		stoploss.ProfitLine.Float64()
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

	if state.Horizon < 0 || state.Observed < 0 {
		return nil, fmt.Errorf("stoploss: stored horizon clock cannot be negative")
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

