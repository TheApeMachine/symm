package broker

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

/*
Recovery reconstructs active positions and working orders from exchange state on boot.
*/
type Recovery struct {
	ctx        context.Context
	cancel     context.CancelFunc
	api        *websocket.API
	ui         chan []byte
	instrument *Instrument
	price      *Price
	balance    *Balance
	recorder   *audit.Recorder
	positions  *sync.Map
}

/*
NewRecovery instantiates a Recovery processor with all broker dependencies.
*/
func NewRecovery(
	ctx context.Context,
	api *websocket.API,
	ui chan []byte,
	instrument *Instrument,
	price *Price,
	balance *Balance,
	recorder *audit.Recorder,
	positions *sync.Map,
) *Recovery {
	ctx, cancel := context.WithCancel(ctx)

	return &Recovery{
		ctx:        ctx,
		cancel:     cancel,
		api:        api,
		ui:         ui,
		instrument: instrument,
		price:      price,
		balance:    balance,
		recorder:   recorder,
		positions:  positions,
	}
}

/*
Recover discovers open inventory, reconciles entry basis from fill history,
and adopts working exits.
*/
func (recovery *Recovery) Recover() error {
	balances, err := recovery.api.Balance()

	if err != nil {
		return errnie.Error(err)
	}

	history, err := recovery.api.TradesHistory()

	if err != nil {
		return errnie.Error(err)
	}

	working, err := recovery.api.OpenOrders()

	if err != nil {
		return errnie.Error(err)
	}

	if err := recovery.cancelBuys(working.Open); err != nil {
		return err
	}

	quote := recovery.api.Normalizer().Name(viper.GetString("market.quote_currency"))

	for asset, amount := range balances {
		asset = recovery.api.Normalizer().Name(asset)

		if asset == "" || asset == quote || amount == nil || amount.Sign() <= 0 {
			continue
		}

		if err := recovery.recoverAsset(asset, quote, amount, history.Trades, working.Open); err != nil {
			return err
		}
	}

	for orderID, order := range working.Open {
		if order.Description != nil && strings.EqualFold(order.Description.Type, "sell") {
			return errnie.Error(errnie.Err(
				errnie.Conflict,
				"desk: working sell "+orderID+" has no reconciled wallet inventory",
				nil,
			))
		}
	}

	return nil
}

func (recovery *Recovery) cancelBuys(orders map[string]spot.Order) error {
	for orderID, order := range orders {
		if order.Description == nil || !strings.EqualFold(order.Description.Type, "buy") {
			continue
		}

		if _, err := uuid.Parse(order.ClOrdID); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Conflict,
				"desk: working buy "+orderID+" is not identifiable as a symm order",
				nil,
			))
		}

		result, err := recovery.api.CancelOrder(&spot.CancelOrderRequest{TxID: orderID})

		if err != nil {
			return errnie.Error(err)
		}

		if result.Count <= 0 && !result.Pending {
			return errnie.Error(errnie.Err(
				errnie.NotFound,
				"desk: working entry "+orderID+" could not be canceled",
				nil,
			))
		}

		return errnie.Error(errnie.Err(
			errnie.NotAcceptable,
			"desk: canceled working entry "+orderID+"; restart after it reaches a terminal state",
			nil,
		))
	}

	return nil
}

func (recovery *Recovery) recoverAsset(
	asset string,
	quote string,
	amount *decimal.Decimal,
	history map[string]spot.Trade,
	working map[string]spot.Order,
) error {
	symbol := asset + "/" + quote
	pair, err := recovery.instrument.Pair(symbol)

	if err != nil {
		return nil
	}

	orderID, order, err := recovery.recoveredSell(pair, working)

	if err != nil {
		return err
	}

	entryPrice, entryFee, entryAt := recovery.recoverBasis(pair, history)

	if entryPrice == nil && order == nil {
		return nil
	}

	quantity, err := recovery.api.Normalizer().FormatSize(symbol, amount)

	if err != nil || quantity == nil || quantity.Sign() <= 0 {
		return nil
	}

	if entryPrice == nil || entryPrice.Sign() <= 0 {
		if tick := recovery.price.Tick(symbol); tick != nil && tick.Last != nil && tick.Last.Sign() > 0 {
			entryPrice = tick.Last
		} else {
			entryPrice = decimal.NewFromInt64(0)
		}
	}

	position := recovery.recoveredPosition(pair, asset, quantity, entryPrice, entryFee, entryAt)

	if order != nil {
		position.adoptExit(orderID, *order)
		delete(working, orderID)
	}

	recovery.positions.Store(symbol, position)
	position.publishSnapshot()
	position.Publish()

	return nil
}

func (recovery *Recovery) recoverBasis(
	pair kraken.InstrumentPair,
	history map[string]spot.Trade,
) (*decimal.Decimal, *decimal.Decimal, time.Time) {
	trades := make([]spot.Trade, 0)
	venue := strings.ToUpper(pair.Base + pair.Quote)

	for _, trade := range history {
		if strings.ToUpper(strings.ReplaceAll(trade.Pair, "/", "")) == venue {
			trades = append(trades, trade)
		}
	}

	if len(trades) == 0 {
		return nil, decimal.NewFromInt64(0), time.Time{}
	}

	sort.Slice(trades, func(left, right int) bool {
		return trades[left].Time.Cmp(trades[right].Time) < 0
	})

	quantity := decimal.NewFromInt64(0)
	cost := decimal.NewFromInt64(0)
	fee := decimal.NewFromInt64(0)
	entryAt := time.Time{}

	for _, trade := range trades {
		if trade.Volume == nil || trade.Cost == nil || trade.Time == nil {
			continue
		}

		if strings.EqualFold(trade.Type, "buy") {
			if quantity.Sign() == 0 {
				entryAt = time.Unix(trade.Time.Int64(), 0).UTC()
			}

			quantity = addAmount(quantity, trade.Volume)
			cost = addAmount(cost, trade.Cost)

			if trade.Fee != nil {
				fee = addAmount(fee, trade.Fee)
			}

			continue
		}

		if strings.EqualFold(trade.Type, "sell") {
			if trade.Volume.Cmp(quantity) >= 0 {
				quantity = decimal.NewFromInt64(0)
				cost = decimal.NewFromInt64(0)
				fee = decimal.NewFromInt64(0)
				entryAt = time.Time{}
				continue
			}

			remaining := subtractAmount(quantity, trade.Volume)
			cost = cost.Mul(remaining).Div(quantity)
			fee = fee.Mul(remaining).Div(quantity)
			quantity = remaining
		}
	}

	if quantity.Sign() > 0 && cost.Sign() > 0 {
		return cost.Div(quantity), fee, entryAt
	}

	return nil, decimal.NewFromInt64(0), time.Time{}
}

func (recovery *Recovery) recoveredPosition(
	pair kraken.InstrumentPair,
	asset string,
	quantity *decimal.Decimal,
	entryPrice *decimal.Decimal,
	entryFee *decimal.Decimal,
	entryAt time.Time,
) *Position {
	position := NewPosition(
		recovery.ctx, recovery.api, recovery.ui, recovery.instrument, recovery.price,
		recovery.balance, recovery.recorder, pair, types.Decision{
			ID:               "recovered:" + pair.Symbol,
			ProposedQuantity: quantity,
			EntryPrice:       entryPrice,
			EntryFee:         entryFee,
			Mark:             entryPrice,
			Risk:             recovery.price.RiskPlan(pair),
		},
	)

	position.Status = types.OPEN
	position.entryTerminal = true
	position.Holding.Asset = asset
	position.Holding.Qty = quantity
	position.Holding.SellableQty = quantity
	position.Holding.EntryAt = &entryAt
	position.Holding.Status = types.OPEN
	position.Holding.Stoploss.BindRecovered()

	return position
}

func (recovery *Recovery) recoveredSell(
	pair kraken.InstrumentPair,
	orders map[string]spot.Order,
) (string, *spot.Order, error) {
	venue := strings.ToUpper(pair.Base + pair.Quote)
	orderID := ""
	var recovered *spot.Order

	for candidateID, order := range orders {
		if order.Description == nil || !strings.EqualFold(order.Description.Type, "sell") {
			continue
		}

		ordered := strings.ToUpper(strings.ReplaceAll(order.Description.Pair, "/", ""))

		if ordered != venue {
			continue
		}

		if recovered != nil {
			return "", nil, errnie.Error(errnie.Err(
				errnie.Conflict,
				"desk: multiple working sells exist for "+pair.Symbol,
				nil,
			))
		}

		copy := order
		orderID = candidateID
		recovered = &copy
	}

	return orderID, recovered, nil
}

func addAmount(left, right *decimal.Decimal) *decimal.Decimal {
	scale := max(left.GetScale(), right.GetScale())
	return left.SetScale(scale).Add(right.SetScale(scale))
}

func subtractAmount(left, right *decimal.Decimal) *decimal.Decimal {
	scale := max(left.GetScale(), right.GetScale())
	return left.SetScale(scale).Sub(right.SetScale(scale))
}

