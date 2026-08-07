package types

import (
	"context"
	"fmt"

	"github.com/theapemachine/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
)

/*
Stoploss regulates one open lot in two regimes.

Before profit lock, Floor is the lowest price implied by the trusted Resonance
path. The distance from entry to that floor is the forecast's own jitter room.
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

/*
NewStoploss constructs the entry regulator from the admitted forecast and the
venue facts needed to state executable price lines.
*/
func NewStoploss(
	ctx context.Context,
	symbol string,
	entryPrice *decimal.Decimal,
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

	if err := stoploss.RebindFill(entryPrice); err != nil {
		cancel()
		return nil, err
	}

	return stoploss, nil
}

/*
RebindFill rebuilds provisional entry geometry from the venue's actual fill.
*/
func (stoploss *Stoploss) RebindFill(entryPrice *decimal.Decimal) error {
	floor, profitLine, armAt, lockFloor, trailDistance, err := stoploss.geometry(
		entryPrice,
	)

	if err != nil {
		return err
	}

	stoploss.Status = ARMED
	stoploss.Mark = entryPrice
	stoploss.Peak = entryPrice
	stoploss.Floor = floor
	stoploss.ProfitLine = profitLine
	stoploss.ArmAt = armAt
	stoploss.LockFloor = lockFloor
	stoploss.trailDistance = trailDistance
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

func (stoploss *Stoploss) geometry(
	entryPrice *decimal.Decimal,
) (
	*decimal.Decimal,
	*decimal.Decimal,
	*decimal.Decimal,
	*decimal.Decimal,
	*decimal.Decimal,
	error,
) {
	if entryPrice == nil || entryPrice.Sign() <= 0 {
		return nil, nil, nil, nil, nil, fmt.Errorf(
			"stoploss: positive entry price required",
		)
	}

	if stoploss.forecast == nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("stoploss: forecast required")
	}

	tick := scaled(stoploss.tickSize)

	if tick == nil || tick.Sign() <= 0 {
		return nil, nil, nil, nil, nil, fmt.Errorf(
			"stoploss: positive tick size required",
		)
	}

	drawdown, err := stoploss.forecast.WorstIntermediateDrawdown()

	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf(
			"stoploss: invalid forecast path: %w", err,
		)
	}

	entry := entryPrice.SetScale(riskScale)
	one := decimal.NewFromInt64(1).SetScale(riskScale)
	survival := one.Sub(decimal.NewFromFloat64(drawdown).SetScale(riskScale))
	floor := floorToTick(entry.Mul(survival), tick)

	if floor == nil || floor.Sign() <= 0 {
		return nil, nil, nil, nil, nil, fmt.Errorf(
			"stoploss: forecast does not imply a positive floor",
		)
	}

	entryRate := scaled(stoploss.entryFeeRate)
	exitRate := scaled(stoploss.exitFeeRate)

	if entryRate == nil || entryRate.Sign() < 0 ||
		exitRate == nil || exitRate.Sign() < 0 || exitRate.Cmp(one) >= 0 {
		return nil, nil, nil, nil, nil, fmt.Errorf(
			"stoploss: valid fee rates required",
		)
	}

	cost := entry.Add(entry.Mul(entryRate))
	breakEven := cost.Div(one.Sub(exitRate))
	profitLine := ceilToTick(breakEven, tick)
	trailDistance := ceilToTick(entry.Sub(floor), tick)

	if trailDistance == nil || trailDistance.Cmp(tick) < 0 {
		trailDistance = tick
	}

	armAt := ceilToTick(
		profitLine.Add(trailDistance).Add(tick),
		tick,
	)
	lockFloor := floorToTick(
		armAt.Sub(trailDistance),
		tick,
	)

	if lockFloor == nil || lockFloor.Cmp(profitLine) <= 0 {
		return nil, nil, nil, nil, nil, fmt.Errorf(
			"stoploss: profit lock must clear profit line",
		)
	}

	return floor, profitLine, armAt, lockFloor, trailDistance, nil
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
