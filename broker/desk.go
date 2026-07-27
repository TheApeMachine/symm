package broker

import (
	"context"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

/*
Desk is the broker Actor entrypoint. It subscribes market topics for
price cache updates and account topics not routed to positions.
Position and Stoploss subscribe directly to the market ticker, account
executions, and account add_order topics so they react to live
websocket messages without Desk absorbing the routing logic.
*/
type Desk struct {
	*types.Actor
	ctx          context.Context
	cancel       context.CancelFunc
	status       types.Status
	api          *websocket.API
	instrument   *Instrument
	price        *Price
	balance      *Balance
	positions    map[string]*Position
	recovered    map[string]*types.Holding
	maxPositions int
	maxReserved  int
	market       *types.Actor
	account      *types.Actor
}

/*
NewDesk constructs the serial broker owner.
*/
func NewDesk(
	ctx context.Context,
	api *websocket.API,
	instrument *Instrument,
	price *Price,
	balance *Balance,
	trading config.TradingConfig,
) *Desk {
	ctx, cancel := context.WithCancel(ctx)

	desk := &Desk{
		ctx:          ctx,
		cancel:       cancel,
		status:       types.INITIALIZING,
		api:          api,
		instrument:   instrument,
		price:        price,
		balance:      balance,
		positions:    make(map[string]*Position),
		recovered:    make(map[string]*types.Holding),
		maxPositions: trading.SlotsNormal,
		maxReserved:  trading.SlotsReserved,
	}

	desk.Actor = types.NewActor(ctx, "desk", map[string]types.Handler{
		"ticker":     {Topic: "ticker", Fn: desk.onTicker},
		"instrument": {Topic: "instrument", Fn: desk.onInstrument},
		"balances":   {Topic: "balances", Fn: desk.onBalances},
	})

	return desk
}

/*
SeedHoldings stores persisted Holding economics by symbol so the single
onBalances recovery path can merge them back into rebuilt Positions once the
wallet snapshot, instruments, and ticker cache are all ready.
*/
func (desk *Desk) SeedHoldings(holdings []*types.Holding) {
	clear(desk.recovered)

	for _, holding := range holdings {
		if holding == nil || holding.Symbol == "" {
			continue
		}

		desk.recovered[holding.Symbol] = holding
	}
}

/*
Initialize attaches Desk to the market and account Actors, subscribes
to shared topics, and publishes the initial balance frame. Position
subscribes to market ticker and account executions/add_order topics
directly so it handles its own lifecycle without Desk absorbing it.
*/
func (desk *Desk) Initialize(market *types.Actor, account *types.Actor) error {
	errnie.Info("initializing desk")

	desk.market = market
	desk.account = account

	topics := []types.Topic{
		{Name: "ticker", Actor: market},
		{Name: "instrument", Actor: market},
	}

	if account != nil {
		topics = append(topics,
			types.Topic{Name: "balances", Actor: account},
		)
	}

	desk.Actor.Initialize(topics...)

	if desk.api != nil {
		if err := desk.api.SubscribeExecutions(); err != nil {
			_ = desk.transitionStatus(types.ERROR)

			return errnie.Error(errnie.Err(
				errnie.Internal,
				"failed to subscribe to executions",
				err,
			))
		}

		if err := desk.hydrateRecovered(); err != nil {
			_ = desk.transitionStatus(types.ERROR)

			return errnie.Error(errnie.Err(
				errnie.Internal,
				"failed to recover account holdings",
				err,
			))
		}
	}

	if err := desk.transitionStatus(types.READY); err != nil {
		return err
	}

	if desk.balance != nil {
		if err := desk.balance.Publish(); err != nil {
			errnie.Error(err)
		}
	}

	return nil
}

func (desk *Desk) hydrateRecovered() error {
	if desk == nil || desk.api == nil {
		return nil
	}

	history, err := desk.api.TradesHistory()

	if err != nil {
		return errnie.Error(err)
	}

	if history == nil {
		return errnie.Error(errnie.Err(
			errnie.NotFound,
			"desk: missing trades history during recovery",
			nil,
		))
	}

	rows := make([]struct {
		symbol string
		trade  spot.Trade
	}, 0, len(history.Result.Trades))

	for _, trade := range history.Result.Trades {
		symbol := desk.api.Name(trade.Pair)

		if symbol == "" {
			return errnie.Error(errnie.Err(
				errnie.NotFound,
				"desk: unknown trade pair during recovery: "+trade.Pair,
				nil,
			))
		}

		rows = append(rows, struct {
			symbol string
			trade  spot.Trade
		}{symbol: symbol, trade: trade})
	}

	sort.Slice(rows, func(left, right int) bool {
		leftAt := time.Time{}
		rightAt := time.Time{}

		if rows[left].trade.Time != nil {
			leftAt = time.Unix(0, int64(rows[left].trade.Time.Float64()*1e9)).UTC()
		}

		if rows[right].trade.Time != nil {
			rightAt = time.Unix(0, int64(rows[right].trade.Time.Float64()*1e9)).UTC()
		}

		return leftAt.Before(rightAt)
	})

	holdings := make(map[string]*types.Holding)
	notionals := make(map[string]*decimal.Decimal)

	for _, row := range rows {
		side := strings.ToLower(row.trade.Type)

		if side == "sell" {
			delete(holdings, row.symbol)
			delete(notionals, row.symbol)
			continue
		}

		if side != "buy" || row.trade.Volume == nil || row.trade.Cost == nil {
			continue
		}

		holding := holdings[row.symbol]

		if holding == nil {
			holding = &types.Holding{Symbol: row.symbol}
			holdings[row.symbol] = holding
		}

		if row.trade.Time != nil {
			entryAt := time.Unix(0, int64(row.trade.Time.Float64()*1e9)).UTC()

			if holding.EntryAt == nil || entryAt.Before(*holding.EntryAt) {
				holding.EntryAt = &entryAt
			}
		}

		if holding.Qty == nil {
			holding.Qty = row.trade.Volume.Copy()
		} else {
			holding.Qty = holding.Qty.Add(row.trade.Volume)
		}

		if row.trade.Fee != nil {
			if holding.EntryFee == nil {
				holding.EntryFee = row.trade.Fee.Copy()
			} else {
				holding.EntryFee = holding.EntryFee.Add(row.trade.Fee)
			}
		}

		if notionals[row.symbol] == nil {
			notionals[row.symbol] = row.trade.Cost.Copy()
		} else {
			notionals[row.symbol] = notionals[row.symbol].Add(row.trade.Cost)
		}

		holding.EntryPrice = notionals[row.symbol].Div(holding.Qty)
	}

	for symbol, recovered := range holdings {
		holding := desk.recovered[symbol]

		if holding == nil {
			holding = &types.Holding{Symbol: symbol}
		}

		holding.EntryAt = recovered.EntryAt
		holding.EntryPrice = recovered.EntryPrice
		holding.EntryFee = recovered.EntryFee

		if holding.Stoploss == nil {
			holding.Stoploss = &types.Stoploss{Symbol: symbol}
		}

		if recovered.EntryPrice != nil {
			holding.Stoploss.Entry = recovered.EntryPrice.Copy()
		}

		desk.recovered[symbol] = holding
	}

	return nil
}

/*
transitionStatus applies one canonical desk lifecycle edge and fails loud on
illegal transitions.
*/
func (desk *Desk) transitionStatus(next types.Status) error {
	status, err := types.Transition(desk.status, next)

	if err != nil {
		return errnie.Error(err)
	}

	desk.status = status
	return nil
}

/*
Status reports desk readiness.
*/
func (desk *Desk) Status() types.Status {
	return desk.status
}

/*
onInstrument forwards the instrument tick then adopts newly visible open lots.
*/
func (desk *Desk) onInstrument(message any) any {
	desk.instrument.On(message)
	desk.adoptOpenPositions()

	return nil
}

/*
onTicker refreshes the price cache and publishes the balance
frame. Position subscribes to the ticker topic directly so it
handles its own mark and stoploss updates without Desk
absorbing that routing logic.
*/
func (desk *Desk) onTicker(message any) any {
	ticker, ok := message.(*kraken.Ticker)

	if !ok {
		ticker = kraken.NewTicker(message.([]byte))
	}

	desk.price.TickerAck(ticker)
	desk.adoptOpenPositions()

	if err := desk.balance.Publish(); err != nil {
		errnie.Error(err)
	}

	return nil
}

/*
onBalances applies the wallet frame and adopts any
wallet-backed lots so restart positions are immediately managed by Desk.
*/
func (desk *Desk) onBalances(message any) any {
	desk.balance.BalanceAck(message.([]byte))
	desk.adoptOpenPositions()

	if err := desk.balance.Publish(); err != nil {
		errnie.Error(err)
	}

	return nil
}

/*
adoptOpenLots recreates desk-owned positions from wallet-backed open lots once
their instrument metadata exists. Balance snapshots can arrive before the
instrument stage is ready, so this helper is called from both balances and
instrument handlers and seeds the recovered position from any cached ticker.
*/
func (desk *Desk) adoptOpenPositions() {
	desk.Update()

	frame := desk.balance.Frame()
	quote := desk.balance.Wallet.quote

	for asset, row := range frame {
		if asset == "" || asset == quote {
			continue
		}

		if row.Balance == nil || row.Balance.Sign() <= 0 {
			continue
		}

		symbol := asset + "/" + quote

		if _, exists := desk.positions[symbol]; exists {
			continue
		}

		pair, err := desk.instrument.Pair(symbol)

		if err != nil {
			continue
		}

		if _, err := desk.price.Get(pair.Symbol); err != nil {
			continue
		}

		position := NewPosition(
			desk.ctx,
			desk.api,
			desk.balance.ui,
			desk.instrument,
			desk.price,
			desk.balance,
			pair,
			row.Balance,
			desk.market,
			desk.account,
		)

		desk.positions[symbol] = position

		desk.recoverPosition(position, desk.recovered[symbol])

		if err := desk.price.Mark(&pair, position.Holding); err != nil {
			delete(desk.positions, symbol)
			continue
		}

		if position.Holding.Mark != nil && position.Holding.Mark.Sign() > 0 &&
			position.Holding.Stoploss != nil {
			position.Holding.Stoploss.Update(position.Holding.Mark)
		}

		position.Publish()
	}
}

/*
recoverPosition merges persisted Holding economics into a rebuilt Position
before the live mark refresh runs. The wallet snapshot remains the source of
truth for quantity; persisted state only restores entry, fees, returns, and the
stop regulator state that balances do not carry.
*/
func (desk *Desk) recoverPosition(position *Position, holding *types.Holding) {
	if position == nil || position.Holding == nil || holding == nil {
		return
	}

	runtimeSymbol := position.Holding.Symbol
	runtimeQty := position.Holding.Qty
	runtimeStatus := position.Holding.Status
	runtimeStoploss := position.Holding.Stoploss
	payload, err := sonic.Marshal(holding)

	if err != nil {
		errnie.Error(err)
		return
	}

	if err := sonic.Unmarshal(payload, position.Holding); err != nil {
		errnie.Error(err)
		return
	}

	position.Holding.Symbol = runtimeSymbol
	position.Holding.Qty = runtimeQty
	position.Holding.Status = runtimeStatus

	if position.Holding.Stoploss == nil {
		position.Holding.Stoploss = runtimeStoploss
	}

	if position.Holding.Stoploss != nil && holding.EntryPrice != nil {
		position.Holding.Stoploss.Entry = holding.EntryPrice.Copy()
	}
}

/*
Positions returns the active desk-managed Position set keyed by symbol so UI
replay can mirror the exact live wire shape after refresh.
*/
func (desk *Desk) Positions() map[string]*Position {
	desk.Update()
	positions := make(map[string]*Position, len(desk.positions))

	for symbol, position := range desk.positions {
		if position == nil {
			continue
		}

		positions[symbol] = position
	}

	return positions
}

/*
Holdings returns the active desk-managed Holding set keyed by symbol. Balance is
wallet-only; open Holding ownership lives on Desk through its Position map.
*/
func (desk *Desk) Holdings() map[string]*types.Holding {
	positions := desk.Positions()
	holdings := make(map[string]*types.Holding, len(positions))

	for symbol, position := range positions {
		if position.Holding == nil {
			continue
		}

		holdings[symbol] = position.Holding
	}

	return holdings
}

/*
Holding returns one active desk-managed Holding by symbol.
*/
func (desk *Desk) Holding(symbol string) (*types.Holding, error) {
	holding, ok := desk.Holdings()[symbol]

	if ok {
		return holding, nil
	}

	return nil, errnie.Error(errnie.Err(
		errnie.NotFound,
		"desk: holding not found",
		nil,
	))
}

/*
Update removes closed or errored positions from the open map.
*/
func (desk *Desk) Update() {
	for symbol, position := range desk.positions {
		if position.Holding.Status == types.CLOSED || slices.Contains([]types.Status{
			types.CLOSED, types.ERROR, types.CANCELED,
		}, position.Status) {
			position.Status = types.CLOSED
			position.Holding.Status = types.CLOSED
			delete(desk.positions, symbol)
		}
	}
}

/*
OpenPositions counts live wallet lots only.
*/
func (desk *Desk) OpenPositions() int {
	desk.Update()
	return len(desk.positions)
}

/*
Position returns the open lot shell for symbol.
*/
func (desk *Desk) Position(symbol string) (*Position, bool) {
	position, ok := desk.positions[symbol]
	return position, ok
}

/*
Balance returns the composed Balance owner.
*/
func (desk *Desk) Balance() *Balance {
	if desk == nil {
		return nil
	}

	return desk.balance
}

/*
Close is retained for boot shutdown symmetry.
*/
func (desk *Desk) Close() error {
	return nil
}

/*
HasSlot reports whether an additional lot may open under slot capacity.
*/
func (desk *Desk) HasSlot(opportunity bool) bool {
	occupied := desk.OpenPositions()

	if !opportunity {
		return occupied < desk.maxPositions
	}

	return occupied < desk.MaxSlots(opportunity)
}

/*
MaxSlots returns normal capacity, optionally including reserved opportunity slots.
*/
func (desk *Desk) MaxSlots(withReserved bool) int {
	if withReserved {
		return desk.maxPositions + desk.maxReserved
	}

	return desk.maxPositions
}

func (desk *Desk) Buy(
	symbol string,
	qty *decimal.Decimal,
	opportunity bool,
) (*Position, error) {
	pair, err := desk.instrument.Pair(symbol)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.NotFound,
			"desk: no instrument for "+symbol,
			err,
		))
	}

	desk.positions[symbol] = NewPosition(
		desk.ctx,
		desk.api,
		desk.balance.ui,
		desk.instrument,
		desk.price,
		desk.balance,
		pair,
		qty,
		desk.market,
		desk.account,
	).Enter()

	return desk.positions[symbol], nil
}

/*
Sell exits the full desk-owned sellable lot for symbol.
*/
func (desk *Desk) Sell(symbol string) error {
	position, ok := desk.Position(symbol)

	if !ok {
		return errnie.Error(errnie.Err(
			errnie.NotFound,
			"desk: no open position for "+symbol,
			nil,
		))
	}

	if err := position.Exit(); err != nil {
		return err
	}

	return nil
}
