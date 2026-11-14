package broker

import (
	"context"
	"encoding/json"
	"iter"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/shopspring/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/nomagique/store"
	"github.com/theapemachine/symm/network"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/ui"
	"github.com/theapemachine/symm/venue"
	"github.com/theapemachine/symm/venue/kraken"
)

/*
TierType selects transport and unlock rules — same Price book, different whether
orders leave the process and whether paper must prove edge first.
*/
type TierType uint8

const (
	TierTypeVirtual TierType = iota
	TierTypePaper
	TierTypeReal
)

/*
String is the cache tierKey and hub JSON label; invalid values log because
corrupt tier keys would write positions under the wrong S3 prefix.
*/
func (tt TierType) String() string {
	switch tt {
	case TierTypeVirtual:
		return "virtual"
	case TierTypePaper:
		return "paper"
	case TierTypeReal:
		return "real"
	}

	errnie.Error(errnie.Err(
		errnie.NotFound,
		"[broker.TierType] invalid tier type",
		nil,
	))

	return ""
}

/*
Tier is one parallel book of positions sharing Price but not cache or socket —
paper proof unlocks disarmed real without mixing fills across lanes.
*/
type Tier struct {
	ctx       context.Context
	Type      TierType
	public    network.Client
	private   network.Client
	price     *Price
	venue     market.Venue
	positions *Cache
	pending   *sync.Map
	balance   *market.Balance
}

/*
NewTier binds venue OnChange to Apply/Hello/history so fills never bypass the
tier cache and balance frames stay aligned with price.balance for Allocate.
*/
func NewTier(
	ctx context.Context,
	tt TierType,
	public network.Client,
	private network.Client,
	price *Price,
	host market.Venue,
	client *store.Client,
) *Tier {
	tier := &Tier{
		ctx:     ctx,
		Type:    tt,
		public:  public,
		private: private,
		price:   price,
		venue:   host,
		pending: &sync.Map{},
		balance: market.NewBalance(map[string]any{
			"quote":  system.Cfg.Market.QuoteCurrency,
			"assets": []*market.Asset{},
		}),
	}

	tier.positions = NewCache(
		ctx, tt.String(), private, price, tier.settle, client,
	)

	if host != nil {
		host.Balance().OnChange(func(frame market.MarketType) {
			frame.ToMarket(tier.balance)

			if tier.price != nil && tier.price.balance != nil {
				frame.ToMarket(tier.price.balance)
			}

			tier.positions.Bound(tier.balance)
			tier.Hello()
		})

		host.TradeBalance().OnChange(func(frame market.MarketType) {
			frame.ToMarket(tier.balance)
			tier.Hello()
		})

		host.Execution().OnChange(func(frame market.MarketType) {
			executions := &market.Execution{}
			frame.ToMarket(executions)

			for _, execution := range executions.Data {
				for _, position := range tier.positions.Open(execution.Symbol) {
					if position == nil {
						continue
					}

					position.Apply(*execution)
				}
			}
		})

		if book, ok := host.(*venue.Kraken); ok && book.Order() != nil {
			book.Order().OnChange(func(frame market.MarketType) {
				order, ok := frame.(*kraken.Order)

				if !ok || order == nil {
					return
				}

				if order.Success {
					tier.ackOrder(order)
					return
				}
			})
		}
	}

	if tier.private != nil && host != nil {
		tier.private.Watch(
			system.Cfg.WebSocket.Endpoints.Balance,
			host.Balance().Request(),
			host.Balance(),
		)
		tier.private.Watch(
			system.Cfg.WebSocket.Endpoints.TradeBalance,
			host.TradeBalance().Request(),
			host.TradeBalance(),
		)
	}

	if client, ok := tier.private.(*network.WebsocketClient); ok {
		_ = client.Open()
	}

	return tier
}

/*
Hello publishes a full positions_snapshot of open positions as an id-keyed map
(hub/UI contract). Marks and PnL are owned by Update — Hello only sums and ships.
*/
func (tier *Tier) Hello() {
	open := tier.positions.Open()
	positions := make(map[string]*Position, len(open))
	unrealized := decimal.Zero

	for _, position := range open {
		if position == nil || position.ID == "" {
			continue
		}

		positions[position.ID] = position

		if position.PnL != nil {
			unrealized = unrealized.Add(*position.PnL)
		}
	}

	tier.balance.Unrealized = &unrealized
	reading := tier.balance.Reading()

	ui.UI.WriteJSON(map[string]any{
		"tier":      tier.Type.String(),
		"positions": positions,
		"balances": map[string]any{
			tier.Type.String(): reading,
		},
		"positions_snapshot": true,
	})
}

/*
Update marks the symbol's open positions, then Hello so the hub always gets a
full positions_snapshot with current PnL / PnLPct — never a sparse delta.
*/
func (tier *Tier) Update(data *market.Update) {
	touched := false

	for _, position := range tier.positions.Open(data.Symbol) {
		if err := position.Update(data.Price); err == nil {
			tier.positions.Save(position)
			touched = true
		}
	}

	if touched {
		tier.Hello()
	}
}

/*
Pending retains flat positions after Remove until strategy records exit_path —
the position is closed in cache but the model still needs the handle for attribution.
*/
func (tier *Tier) Pending() iter.Seq[*Position] {
	return func(yield func(*Position) bool) {
		tier.pending.Range(func(_, value any) bool {
			return yield(value.(*Position))
		})
	}
}

/*
ClearPending lets the model acknowledge exit-path attribution after stoploss
settle without deleting the closed record from cache.
*/
func (tier *Tier) ClearPending(symbol string) {
	if symbol == "" {
		return
	}

	tier.pending.Delete(symbol)
}

/*
Enter binds spot wallet to Allocate so mins and cash match what Send will spend;
transport-less tiers immediately fillsEnter so tests do not need fake ws acks.
*/
func (tier *Tier) Enter(
	symbol string, path string, pathFound bool,
) error {
	if tier.positions.Has(symbol) {
		return errnie.Err(
			errnie.TooManyRequests,
			"[broker.Tier.Enter] position already exists",
			nil,
		)
	}

	qty, err := tier.price.Allocate(symbol, tier.balance)

	if err != nil || qty == nil || qty.Sign() <= 0 {
		if err != nil {
			return errnie.Error(err)
		}

		return errnie.Err(
			errnie.UnprocessableContent,
			"[broker.Tier.Enter] cannot size "+symbol,
			nil,
		)
	}

	position := NewPosition(
		tier.ctx,
		symbol,
		tier.Type.String(),
		path,
		tier.price,
		tier.private,
		tier.settle,
	)

	if err := position.Enter(path, qty, tier.balance); err != nil {
		return errnie.Error(err)
	}

	tier.positions.Add(symbol, position, true)

	if err := tier.Send(position, "enter"); err != nil {
		return errnie.Error(err)
	}

	return nil
}

/*
Exit arms Closing then Send (or fillsExit without transport) so policy exits share
the same sell path as stoploss settle rather than mutating cache timestamps alone.
*/
func (tier *Tier) Exit(
	position *Position,
) error {
	if err := position.exit(); err != nil {
		return errnie.Error(err)
	}

	if err := tier.Send(position, "exit"); err != nil {
		// Keep Closing: Write may have delivered the sell. Clearing here
		// allows a second Send while the first fill is in flight.
		return errnie.Error(err)
	}

	if err := tier.positions.Save(position); err != nil {
		return errnie.Error(err)
	}

	return nil
}

/*
apply the execution to the position
*/
func (tier *Tier) apply(execution *market.Execution) {
	for _, position := range tier.positions.All() {
		if position == nil {
			continue
		}

		for _, data := range execution.Data {
			if data.Symbol == position.Symbol {
				position.Apply(*data)
			}
		}

		if err := tier.positions.Save(position); err != nil {
			errnie.Error(err)
		}
	}
}

/*
settle the position.
*/
func (tier *Tier) settle(position *Position) {
	if position == nil {
		return
	}

	tier.positions.Remove(position.Symbol)
	tier.Hello()
}

/*
Send is the only wire exit for real orders; token failures drop pending enters
but clear exit cl_ord_id so kickExit can remint after auth heals.
*/
func (tier *Tier) Send(position *Position, action string) error {
	order := position.EntryOrder

	if action == "exit" {
		order = position.ExitOrder
	}

	if tier.private == nil || order == nil {
		return nil
	}

	params := map[string]any{
		"symbol":     position.Symbol,
		"side":       order.Type,
		"order_type": order.OrderType,
		"order_qty":  json.Number(order.Volume),
		"cl_ord_id":  order.ClOrdId,
	}

	if tier.Type == TierTypeReal {
		token, err := tier.price.Instrument.auth.Token(tier.ctx)

		if err != nil {
			return errnie.Error(err)
		}

		params["token"] = token
	}

	buf, err := sonic.Marshal(map[string]any{
		"method": "add_order",
		"params": params,
	})

	if err != nil {
		return errnie.Err(
			errnie.Internal,
			"[broker.Tier.Send] failed to marshal message",
			err,
		)
	}

	if err := tier.private.Write(buf); err != nil {
		return errnie.Error(err)
	}

	return nil
}

/*
ackOrder records venue order_id on the position so fills without cl_ord_id still Owns.
*/
func (tier *Tier) ackOrder(order *kraken.Order) {
	clOrdID := order.Result.ClOrdID
	orderID := order.Result.OrderID

	if clOrdID == "" || orderID == "" {
		return
	}

	for _, position := range tier.positions.Open() {
		if position == nil || !position.Closing {
			continue
		}

		position.mu.Lock()

		switch {
		case position.EntryOrder != nil && position.EntryOrder.ClOrdId == clOrdID:
			position.EntryVenueOrderID = orderID
			position.mu.Unlock()
			return

		case position.ExitOrder != nil && position.ExitOrder.ClOrdId == clOrdID:
			position.ExitVenueOrderID = orderID
			position.mu.Unlock()
			return
		}

		position.mu.Unlock()
	}
}

/*
Run replays settle for positions restored mid-close after restart so a process bounce
does not strand Closing positions without a private reader attached.
*/
func (tier *Tier) Run() {
	for {
		select {
		case <-tier.ctx.Done():
			return
		default:
		}

		buf, err := tier.private.Read()

		if err != nil {
			if tier.ctx.Err() != nil {
				return
			}

			if !network.Recoverable(err) {
				errnie.Error(errnie.Err(
					errnie.IO, "[tier] private ingress read failed", err,
				))
			}

			continue
		}

		tier.venue.Read(buf)
	}
}
