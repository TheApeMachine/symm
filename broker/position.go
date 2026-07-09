package broker

import (
	"math"
	"math/big"
	"strings"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

var statusMap = map[string]types.Status{
	"new":              types.NEW,
	"open":             types.OPEN,
	"pending":          types.PENDING,
	"partially_filled": types.PARTIAL,
	"filled":           types.FILLED,
	"canceled":         types.CANCELED,
	"expired":          types.ERROR,
}

type PositionData struct {
	Symbol     string          `json:"symbol"`
	Qty        float64         `json:"qty"`
	EntryPrice decimal.Decimal `json:"entry_price"`
	Mark       decimal.Decimal `json:"mark"`
	PnL        decimal.Decimal `json:"pnl"`
	ReturnPct  float64         `json:"return_pct"`
	// FeeRate is the real per-symbol taker fee fraction for this position, from
	// the live Kraken fee schedule. Surfaced so downstream consumers (e.g. the
	// portfolio's round-trip friction floor) use real fees, never a config guess.
	FeeRate        float64         `json:"fee_rate"`
	Spread         decimal.Decimal `json:"spread"`
	PriceIncrement decimal.Decimal `json:"price_increment"`
}

type positionMark struct {
	price     decimal.Decimal
	pnl       decimal.Decimal
	returnPct float64
}

type Position struct {
	status     types.Status
	private    websocket.Private
	orderID    string
	order      *kraken.OrderData
	executions []*kraken.ExecutionData
	data       *PositionData
	tickers    []*kraken.TickerData
	feeRate    float64
	closing    bool
}

func NewPosition(
	private websocket.Private,
	data *PositionData,
) *Position {
	return &Position{
		status:     types.INITIALIZING,
		private:    private,
		data:       data,
		executions: make([]*kraken.ExecutionData, 0),
		tickers:    make([]*kraken.TickerData, 0),
	}
}

func (position *Position) OrderAck(orderAck *kraken.OrderResponse) error {
	position.status = types.OPEN
	return nil
}

func (position *Position) SetFeeRate(rate float64) {
	position.feeRate = rate
	position.data.FeeRate = rate
}

func (position *Position) Order(order *kraken.OrderData) error {
	position.order = order

	if status, ok := statusMap[order.Status]; ok {
		position.status = status
		return nil
	}

	if position.status == types.INITIALIZING {
		position.status = types.OPEN
	}

	return nil
}

func (position *Position) Execution(execution *kraken.ExecutionData) error {
	position.executions = append(position.executions, execution)

	if strings.EqualFold(execution.PositionStatus, "open") {
		position.status = types.OPEN
		position.closing = false

		if execution.LastQty > 0 && execution.LastQty != position.data.Qty {
			position.data.Qty = execution.LastQty
		}

		avgPriceRat := execution.AvgPrice.Rat()
		if avgPriceRat.Sign() > 0 &&
			position.data.EntryPrice.Rat().Cmp(avgPriceRat) != 0 {
			position.data.EntryPrice = execution.AvgPrice
		}

		return nil
	}

	if strings.EqualFold(execution.Side, "sell") &&
		strings.EqualFold(execution.OrderStatus, "filled") {
		position.status = types.CLOSED
		return nil
	}

	if status, ok := statusMap[execution.OrderStatus]; ok {
		position.status = status
	}

	return nil
}

func (position *Position) AddTicker(ticker *kraken.TickerData) error {
	position.tickers = append(position.tickers, ticker)

	if math.IsNaN(position.feeRate) || math.IsInf(position.feeRate, 0) || position.feeRate < 0 {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"position: fee rate must be finite and non-negative",
			nil,
		))
	}

	mark, err := position.mark(ticker)

	if err != nil || mark == nil {
		return err
	}

	position.data.Mark = mark.price
	position.data.PnL = mark.pnl
	position.data.ReturnPct = mark.returnPct

	askRat := ticker.Ask.Rat()
	bidRat := ticker.Bid.Rat()
	two := big.NewRat(2, 1)
	midRat := new(big.Rat).Quo(new(big.Rat).Add(askRat, bidRat), two)

	if midRat.Sign() > 0 {
		spreadRat := new(big.Rat).Quo(new(big.Rat).Sub(askRat, bidRat), midRat)
		spreadDec, err := decimal.NewFromString(spreadRat.FloatString(8))

		if err == nil {
			position.data.Spread = *spreadDec
		}
	}

	return nil
}

func (position *Position) mark(ticker *kraken.TickerData) (*positionMark, error) {
	entry := position.data.EntryPrice
	last := ticker.Bid

	if last.Rat().Sign() <= 0 {
		last = ticker.Last
	}

	if entry.Rat().Sign() <= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"position: entry price required to mark position",
			nil,
		))
	}

	if last.Rat().Sign() <= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"position: ticker price required to mark position",
			nil,
		))
	}

	qty := decimal.NewFromFloat64(position.data.Qty)
	qtyRat := qty.Rat()

	if qtyRat.Sign() <= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"position: quantity required to mark position",
			nil,
		))
	}

	entryRat := entry.Rat()
	lastRat := last.Rat()

	calculationScale := position.scale(entry, last, *qty)
	realizedFee, err := position.fees(entryRat, qtyRat, calculationScale)

	if err != nil {
		return nil, err
	}

	grossRat := new(big.Rat).Mul(
		new(big.Rat).Sub(lastRat, entryRat),
		qtyRat,
	)
	exitFeeRat := new(big.Rat).Quo(
		new(big.Rat).Mul(realizedFee.Rat(), lastRat),
		entryRat,
	)
	netRat := new(big.Rat).Sub(
		new(big.Rat).Sub(grossRat, realizedFee.Rat()),
		exitFeeRat,
	)
	denominator := new(big.Rat).Mul(entryRat, qtyRat)
	returnPct, _ := new(big.Rat).Quo(netRat, denominator).Float64()
	net, err := decimal.NewFromString(netRat.FloatString(calculationScale))

	if err != nil || math.IsNaN(returnPct) || math.IsInf(returnPct, 0) {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"position: invalid mark-to-market calculation",
			err,
		))
	}

	return &positionMark{
		price:     last,
		pnl:       *net,
		returnPct: returnPct,
	}, nil
}

func (position *Position) scale(
	entry decimal.Decimal,
	last decimal.Decimal,
	qty decimal.Decimal,
) int {
	return int(max(
		entry.GetScale(),
		last.GetScale(),
		qty.GetScale(),
		decimal.NewFromFloat64(position.feeRate).GetScale(),
	))
}

/*
fees returns the entry fee from the account's real TradeVolume taker rate.
*/
func (position *Position) fees(
	entryRat *big.Rat,
	qtyRat *big.Rat,
	calculationScale int,
) (*decimal.Decimal, error) {
	if position.feeRate == 0 {
		return decimal.NewFromFloat64(0), nil
	}

	if math.IsNaN(position.feeRate) || math.IsInf(position.feeRate, 0) || position.feeRate < 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"position: fee rate must be finite and non-negative",
			nil,
		))
	}

	feeRate := decimal.NewFromFloat64(position.feeRate)
	feeRat := new(big.Rat).Mul(
		new(big.Rat).Mul(entryRat, qtyRat),
		feeRate.Rat(),
	)
	estimated, err := decimal.NewFromString(feeRat.FloatString(calculationScale))

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"position: invalid fee calculation",
			err,
		))
	}

	return estimated, nil
}

func (position *Position) Enter() error {
	position.closing = false
	err := errnie.Error(
		position.private.Submit(&kraken.Order{
			Method: "add_order",
			Params: kraken.LimitOrderParams{
				OrderType: "market",
				Side:      "buy",
				OrderQty:  position.data.Qty,
				Symbol:    position.data.Symbol,
			},
			ReqID: int(time.Now().UnixNano()),
		}),
	)

	if err != nil {
		position.status = types.ERROR
		return err
	}

	position.status = types.PENDING
	return nil
}

func (position *Position) Exit() error {
	position.closing = true
	err := position.private.Submit(&kraken.Order{
		Method: "add_order",
		Params: kraken.LimitOrderParams{
			OrderType: "market",
			Side:      "sell",
			OrderQty:  position.data.Qty,
			Symbol:    position.data.Symbol,
		},
		ReqID: int(time.Now().UnixNano()),
	})

	if err != nil {
		position.closing = false
		position.status = types.ERROR
		return err
	}

	position.status = types.PENDING
	return nil
}
