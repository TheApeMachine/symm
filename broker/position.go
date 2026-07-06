package broker

import (
	"strings"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
)

type PositionData struct {
	Symbol     string  `json:"symbol"`
	Qty        float64 `json:"qty"`
	EntryPrice float64 `json:"entry_price"`
	Mark       float64 `json:"mark"`
	PnL        float64 `json:"pnl"`
	ReturnPct  float64 `json:"return_pct"`
}

type Position struct {
	private websocket.Private
	mu      sync.RWMutex
	data    PositionData
	Symbol  string
	Qty     float64
}

func NewPosition(
	private websocket.Private,
	balance *kraken.BalanceDataSlice,
	symbol string,
	fraction float64,
	price float64,
) (*Position, error) {
	symbol = strings.TrimSpace(symbol)
	_, quote, ok := strings.Cut(symbol, "/")

	if !ok || strings.TrimSpace(quote) == "" {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"broker: buy symbol must include base and quote",
			nil,
		))
	}

	if fraction <= 0 || fraction > 1 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"broker: buy fraction must be within the quote balance",
			nil,
		))
	}

	if price <= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"broker: buy price must be positive",
			nil,
		))
	}

	if balance == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"broker: balance required",
			nil,
		))
	}

	notional := 0.0
	for _, row := range *balance {
		if strings.EqualFold(row.Asset, quote) {
			notional = row.Available * fraction
			break
		}
	}

	if notional <= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"broker: buy quote balance must be positive",
			nil,
		))
	}

	return &Position{
		private: private,
		data: PositionData{
			Symbol:     symbol,
			Qty:        notional / price,
			EntryPrice: price,
			Mark:       price,
		},
	}, nil
}

func (position *Position) Data() PositionData {
	position.mu.RLock()
	defer position.mu.RUnlock()

	if position.data.Symbol == "" {
		return PositionData{
			Symbol: position.Symbol,
			Qty:    position.Qty,
		}
	}

	return position.data
}

func (position *Position) Update(ticker kraken.TickerData) {
	if strings.TrimSpace(ticker.Symbol) != position.Data().Symbol {
		return
	}

	mark := ticker.Last
	if mark <= 0 && ticker.Bid > 0 && ticker.Ask > 0 {
		mark = (ticker.Bid + ticker.Ask) / 2
	}

	if mark <= 0 {
		return
	}

	position.mu.Lock()
	defer position.mu.Unlock()

	position.data.Mark = mark
	position.data.PnL = (mark - position.data.EntryPrice) * position.data.Qty

	if position.data.EntryPrice > 0 {
		position.data.ReturnPct = mark/position.data.EntryPrice - 1
	}
}

func (position *Position) Enter() error {
	data := position.Data()

	return position.private.Submit(&kraken.Order{
		Method: "add_order",
		Params: kraken.LimitOrderParams{
			OrderType: "market",
			Side:      "buy",
			OrderQty:  data.Qty,
			Symbol:    data.Symbol,
		},
		ReqID: int(time.Now().UnixNano()),
	})
}

func (position *Position) Exit() error {
	data := position.Data()

	return position.private.Submit(&kraken.Order{
		Method: "add_order",
		Params: kraken.LimitOrderParams{
			OrderType: "market",
			Side:      "sell",
			OrderQty:  data.Qty,
			Symbol:    data.Symbol,
		},
		ReqID: int(time.Now().UnixNano()),
	})
}
