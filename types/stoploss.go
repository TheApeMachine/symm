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

/*
Stoploss regulates one open lot in two regimes.

Before profit lock, Floor is the lower of the trusted Resonance path boundary
and the boundary outside the lot's measured round-trip execution cost. A move
smaller than the cost of entering and exiting is not adverse evidence.
Once Mark clears the profit line by that distance plus one executable tick,
Floor moves above ProfitLine. Every later new Peak raises Floor by the same
distance; no path lowers it.

A profitable mark that fails to make a new peak is stagnant: the lot is
no longer working, so Update sells it instead of waiting for a pullback
to the floor.
*/
type Stoploss struct {
	ctx           context.Context
	cancel        context.CancelFunc
	forecast      *learning.RLSOutput
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

	if err := stoploss.RebindFill(entryPrice, mark); err != nil {
		cancel()
		return nil, err
	}

	return stoploss, nil
}

/*
RebindFill updates the lot from its realized entry and current executable mark.
The forecast room is retained, but never allowed to be narrower than the
measured distance from that mark to fee-inclusive break-even.
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

	if stoploss.Locked && raisedPeak {
		candidate := floorToTick(
			scaled(mark).Sub(stoploss.trailDistance),
			stoploss.tickSize,
		)

		if candidate != nil && candidate.Cmp(stoploss.Floor) > 0 {
			stoploss.Floor = candidate
		}
	}

	if !raisedPeak && stoploss.ProfitLine != nil &&
		mark.Cmp(stoploss.ProfitLine) > 0 {
		stoploss.Status = TRIGGERED
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

	if state.Floor.Cmp(state.Peak) >= 0 {
		return nil, fmt.Errorf("stoploss: stored floor must remain below peak")
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

	minimumPathReturn := stoploss.forecast.Value
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

	// A path with no predicted dip reaches the current mark. The strict stop
	// lattice projects that boundary to the immediately preceding venue tick.
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

	if tick == nil || tick.Sign() <= 0 || stoploss.trailDistance == nil {
		return nil, nil, nil, fmt.Errorf("stoploss: forecast geometry required")
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
	executionDistance := ceilToTick(profitLine.Sub(currentMark), tick)
	trailDistance := largest(stoploss.trailDistance, executionDistance)
	executionFloor := floorToTick(currentMark.Sub(trailDistance), tick)

	if executionFloor == nil || executionFloor.Sign() <= 0 {
		return nil, nil, nil, fmt.Errorf(
			"stoploss: execution cost does not imply a positive floor",
		)
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
		return nil, nil, nil, fmt.Errorf(
			"stoploss: profit lock must clear profit line",
		)
	}

	if executionFloor.Cmp(stoploss.Floor) < 0 {
		stoploss.Floor = executionFloor
	}

	stoploss.trailDistance = trailDistance

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
