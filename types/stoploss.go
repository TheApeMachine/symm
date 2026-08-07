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
path. Once Mark clears ArmAt, Floor moves immediately to LockFloor. Every later
new Peak may raise Floor by the RiskPlan trail distance; no path lowers it.
*/
type Stoploss struct {
	ctx        context.Context
	cancel     context.CancelFunc
	forecast   *ResonanceForecast
	risk       RiskPlan
	Status     Status           `json:"status"`
	Symbol     string           `json:"symbol"`
	Floor      *decimal.Decimal `json:"floor"`
	Mark       *decimal.Decimal `json:"mark"`
	Peak       *decimal.Decimal `json:"peak"`
	ProfitLine *decimal.Decimal `json:"profit_line"`
	ArmAt      *decimal.Decimal `json:"arm_at"`
	LockFloor  *decimal.Decimal `json:"lock_floor"`
	Locked     bool             `json:"locked"`
}

/*
NewStoploss constructs the entry regulator from the exact forecast and risk
geometry carried by the strategy decision.
*/
func NewStoploss(
	ctx context.Context,
	symbol string,
	entryPrice *decimal.Decimal,
	forecast *ResonanceForecast,
	risk RiskPlan,
) (*Stoploss, error) {
	if symbol == "" {
		return nil, fmt.Errorf("stoploss: symbol required")
	}

	ctx, cancel := context.WithCancel(ctx)
	stoploss := &Stoploss{
		ctx:      ctx,
		cancel:   cancel,
		forecast: forecast,
		risk:     risk,
		Status:   ARMED,
		Symbol:   symbol,
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
	floor, profitLine, armAt, lockFloor, err := stoploss.geometry(entryPrice)

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
		mark.SetScale(riskScale).Sub(stoploss.risk.TrailDistance),
		stoploss.risk.TickSize,
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
	error,
) {
	if entryPrice == nil || entryPrice.Sign() <= 0 {
		return nil, nil, nil, nil, fmt.Errorf("stoploss: positive entry price required")
	}

	if stoploss.forecast == nil || !stoploss.risk.Present {
		return nil, nil, nil, nil, fmt.Errorf("stoploss: forecast and risk plan required")
	}

	if stoploss.risk.RiskDistance == nil || stoploss.risk.RiskDistance.Sign() <= 0 ||
		stoploss.risk.TrailDistance == nil || stoploss.risk.TrailDistance.Sign() <= 0 ||
		stoploss.risk.ArmBuffer == nil || stoploss.risk.ArmBuffer.Sign() <= 0 ||
		stoploss.risk.LockBuffer == nil || stoploss.risk.LockBuffer.Sign() <= 0 ||
		stoploss.risk.MinEdge == nil || stoploss.risk.MinEdge.Sign() <= 0 {
		return nil, nil, nil, nil, fmt.Errorf("stoploss: incomplete risk geometry")
	}

	drawdown, err := stoploss.forecast.WorstIntermediateDrawdown()

	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("stoploss: invalid forecast path: %w", err)
	}

	entry := entryPrice.SetScale(riskScale)
	one := decimal.NewFromInt64(1).SetScale(riskScale)
	survival := one.Sub(decimal.NewFromFloat64(drawdown).SetScale(riskScale))
	floor := floorToTick(entry.Mul(survival), stoploss.risk.TickSize)
	hardFloor := floorToTick(
		entry.Sub(stoploss.risk.RiskDistance),
		stoploss.risk.TickSize,
	)

	if floor == nil || floor.Sign() <= 0 || hardFloor == nil || floor.Cmp(hardFloor) < 0 {
		return nil, nil, nil, nil, fmt.Errorf(
			"stoploss: forecast drawdown exceeds sized risk geometry",
		)
	}

	entryRate := scaled(stoploss.risk.EntryFeeRate)
	exitRate := scaled(stoploss.risk.ExitFeeRate)

	if entryRate == nil || entryRate.Sign() < 0 ||
		exitRate == nil || exitRate.Sign() < 0 || exitRate.Cmp(one) >= 0 {
		return nil, nil, nil, nil, fmt.Errorf("stoploss: valid fee rates required")
	}

	cost := entry.Add(entry.Mul(entryRate))
	breakEven := cost.Div(one.Sub(exitRate))
	profitLine := ceilToTick(
		breakEven.Add(stoploss.risk.MinEdge),
		stoploss.risk.TickSize,
	)
	armAt := ceilToTick(
		profitLine.Add(stoploss.risk.ArmBuffer),
		stoploss.risk.TickSize,
	)
	lockFloor := ceilToTick(
		profitLine.Add(stoploss.risk.LockBuffer),
		stoploss.risk.TickSize,
	)

	return floor, profitLine, armAt, lockFloor, nil
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
