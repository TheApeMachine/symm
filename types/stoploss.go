package types

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/theapemachine/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
)

/*
Stoploss regulates one open lot in two regimes.

Before profit lock, Floor is the lowest executable mark implied by the trusted
Resonance path. The distance from the current mark to that floor is the
forecast's own jitter room.
Once Mark clears the profit line by that distance plus one executable tick,
Floor moves above ProfitLine. Every later new Peak raises Floor by the same
distance; no path lowers it.
*/
type Stoploss struct {
	ctx           context.Context
	cancel        context.CancelFunc
	forecast      *ResonanceForecast
	tickSize      *decimal.Decimal
	entryFeeRate  *decimal.Decimal
	exitFeeRate   *decimal.Decimal
	trailDistance *decimal.Decimal
	Status        Status           `json:"status"`
	Symbol        string           `json:"symbol"`
	Floor         *decimal.Decimal `json:"floor"`
	Mark          *decimal.Decimal `json:"mark"`
	Peak          *decimal.Decimal `json:"peak"`
	ProfitLine    *decimal.Decimal `json:"profit_line"`
	ArmAt         *decimal.Decimal `json:"arm_at"`
	LockFloor     *decimal.Decimal `json:"lock_floor"`
	Locked        bool             `json:"locked"`
}

type stoplossState struct {
	Status        Status           `json:"status"`
	Symbol        string           `json:"symbol"`
	TickSize      *decimal.Decimal `json:"tick_size"`
	TrailDistance *decimal.Decimal `json:"trail_distance"`
	Floor         *decimal.Decimal `json:"floor"`
	Mark          *decimal.Decimal `json:"mark"`
	Peak          *decimal.Decimal `json:"peak"`
	ProfitLine    *decimal.Decimal `json:"profit_line"`
	ArmAt         *decimal.Decimal `json:"arm_at"`
	LockFloor     *decimal.Decimal `json:"lock_floor"`
	Locked        bool             `json:"locked"`
}

/*
NewStoploss constructs the entry regulator from the admitted forecast and the
venue facts needed to state executable price lines.
*/
func NewStoploss(
	ctx context.Context,
	symbol string,
	entryPrice *decimal.Decimal,
	mark *decimal.Decimal,
	forecast *ResonanceForecast,
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
	}

	floor, trailDistance, err := stoploss.forecastGeometry(mark)

	if err != nil {
		cancel()
		return nil, err
	}

	stoploss.Mark = mark
	stoploss.Peak = mark
	stoploss.Floor = floor
	stoploss.trailDistance = trailDistance

	if err := stoploss.RebindFill(entryPrice); err != nil {
		cancel()
		return nil, err
	}

	return stoploss, nil
}

/*
RebindFill updates fill-dependent profit geometry without changing the
forecast floor or its remembered reach.
*/
func (stoploss *Stoploss) RebindFill(entryPrice *decimal.Decimal) error {
	profitLine, armAt, lockFloor, err := stoploss.entryGeometry(entryPrice)

	if err != nil {
		return err
	}

	stoploss.Status = ARMED
	stoploss.ProfitLine = profitLine
	stoploss.ArmAt = armAt
	stoploss.LockFloor = lockFloor
	stoploss.Locked = false

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

	if mark.Cmp(stoploss.Floor) < 0 {
		stoploss.Status = TRIGGERED
		return
	}

	raisedPeak := mark.Cmp(stoploss.Peak) > 0

	if raisedPeak {
		stoploss.Peak = mark
	}

	if !stoploss.Locked && mark.Cmp(stoploss.ArmAt) >= 0 {
		stoploss.Locked = true

		if stoploss.LockFloor.Cmp(stoploss.Floor) > 0 {
			stoploss.Floor = stoploss.LockFloor
		}

		return
	}

	if !stoploss.Locked || !raisedPeak {
		return
	}

	candidate := floorToTick(
		mark.SetScale(riskScale).Sub(stoploss.trailDistance),
		stoploss.tickSize,
	)

	if candidate != nil && candidate.Cmp(stoploss.Floor) > 0 {
		stoploss.Floor = candidate
	}
}

/*
MarshalState encodes the live values needed to continue regulating the lot.
*/
func (stoploss *Stoploss) MarshalState() ([]byte, error) {
	if stoploss == nil {
		return nil, fmt.Errorf("stoploss: state required")
	}

	return json.Marshal(stoplossState{
		Status:        stoploss.Status,
		Symbol:        stoploss.Symbol,
		TickSize:      stoploss.tickSize,
		TrailDistance: stoploss.trailDistance,
		Floor:         stoploss.Floor,
		Mark:          stoploss.Mark,
		Peak:          stoploss.Peak,
		ProfitLine:    stoploss.ProfitLine,
		ArmAt:         stoploss.ArmAt,
		LockFloor:     stoploss.LockFloor,
		Locked:        stoploss.Locked,
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

	ctx, cancel := context.WithCancel(ctx)

	return &Stoploss{
		ctx:           ctx,
		cancel:        cancel,
		tickSize:      state.TickSize,
		trailDistance: state.TrailDistance,
		Status:        state.Status,
		Symbol:        state.Symbol,
		Floor:         state.Floor,
		Mark:          state.Mark,
		Peak:          state.Peak,
		ProfitLine:    state.ProfitLine,
		ArmAt:         state.ArmAt,
		LockFloor:     state.LockFloor,
		Locked:        state.Locked,
	}, nil
}

func (stoploss *Stoploss) forecastGeometry(
	mark *decimal.Decimal,
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

	drawdown, err := stoploss.forecast.WorstIntermediateDrawdown()

	if err != nil {
		return nil, nil, fmt.Errorf(
			"stoploss: invalid forecast path: %w", err,
		)
	}

	currentMark := mark.SetScale(riskScale)
	one := decimal.NewFromInt64(1).SetScale(riskScale)
	survival := one.Sub(decimal.NewFromFloat64(drawdown).SetScale(riskScale))
	floor := floorToTick(currentMark.Mul(survival), tick)

	if floor == nil || floor.Sign() <= 0 {
		return nil, nil, fmt.Errorf(
			"stoploss: forecast does not imply a positive floor",
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
) (*decimal.Decimal, *decimal.Decimal, *decimal.Decimal, error) {
	if entryPrice == nil || entryPrice.Sign() <= 0 {
		return nil, nil, nil, fmt.Errorf(
			"stoploss: positive entry price required",
		)
	}

	tick := scaled(stoploss.tickSize)

	if tick == nil || tick.Sign() <= 0 || stoploss.trailDistance == nil {
		return nil, nil, nil, fmt.Errorf("stoploss: forecast geometry required")
	}

	entry := entryPrice.SetScale(riskScale)
	one := decimal.NewFromInt64(1).SetScale(riskScale)
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

	armAt := ceilToTick(
		profitLine.Add(stoploss.trailDistance).Add(tick),
		tick,
	)
	lockFloor := floorToTick(
		armAt.Sub(stoploss.trailDistance),
		tick,
	)

	if lockFloor == nil || lockFloor.Cmp(profitLine) <= 0 {
		return nil, nil, nil, fmt.Errorf(
			"stoploss: profit lock must clear profit line",
		)
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
