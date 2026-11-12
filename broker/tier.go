package broker

import (
	"context"
	"encoding/json"
	"iter"
	"sync"
	"sync/atomic"
	"time"

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
corrupt tier keys would write lots under the wrong S3 prefix.
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
	WinRate   float64
	Edge      float64
	public    network.Client
	private   network.Client
	price     *Price
	venue     market.Venue
	positions *Cache
	pending   *sync.Map
	unlocked  bool
	live      bool
	exchange  struct {
		equity     float64
		unrealized float64
		ok         bool
	}
	exchangeClosed  sync.Map
	historyRunning  atomic.Bool
	historyMu       sync.Mutex
	historySyncedAt time.Time
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
	live := system.Cfg.Market.Model == "real"

	// Real orders leave the process only when market.model is real.
	if tt == TierTypeReal && !live {
		private = nil
	}

	tier := &Tier{
		ctx:     ctx,
		Type:    tt,
		public:  public,
		private: private,
		price:   price,
		venue:   host,
		pending: &sync.Map{},
		live:    live,
	}

	tier.positions = NewCache(
		ctx, tt.String(), private, price, tier.settle, client,
	)

	if host != nil {
		host.Balance().OnChange(func(frame market.MarketType) {
			// BookIntact / CanSize size through price.balance; keep it
			// on the same spot ledger Enter spends (not paper-only).
			if tier.price != nil && tier.price.balance != nil {
				frame.ToMarket(tier.price.balance)
			}

			tier.Hello()

			if tier.Type == TierTypeReal {
				go tier.pullHistory(false)
			}
		})

		host.TradeBalance().OnChange(func(frame market.MarketType) {
			if eq, un, ok := tier.exchangeBalanceFrom(frame); ok {
				tier.exchange.equity = eq
				tier.exchange.unrealized = un
				tier.exchange.ok = true
			}

			tier.Hello()
		})

		host.Execution().OnChange(func(frame market.MarketType) {
			execution := &market.Execution{}
			frame.ToMarket(execution)
			tier.apply(execution)
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

				tier.rejectOrder(order)
			})
		}

	}

	tier.Recalculate()
	tier.unlocked = tier.Unlocked(nil)

	return tier
}

/*
apply is the only execution ingress: unmatched prints are ignored, matched lots
persist before kickExit so a resend sees saved Closing state.
*/
func (tier *Tier) apply(executions *market.Execution) {
	if executions == nil {
		return
	}

	touched := make(map[string]*Position)

	for _, execution := range executions.Data {
		if execution == nil || !tier.fillReport(execution) {
			continue
		}

		matched := false

		for _, position := range tier.positions.Submitted(execution.Symbol) {
			if position == nil || !position.Owns(*execution) {
				continue
			}

			position.Apply(*execution)
			touched[position.ID] = position
			matched = true
		}

		if matched {
			continue
		}

		for _, position := range tier.positions.Closed(execution.Symbol) {
			if position == nil || !position.Owns(*execution) {
				continue
			}

			position.Apply(*execution)
			touched[position.ID] = position
		}
	}

	if len(touched) == 0 {
		return
	}

	closed := false

	for _, position := range touched {
		if err := tier.positions.Save(position); err != nil {
			errnie.Error(err)
		}

		tier.kickExit(position)

		if position != nil && position.ClosedAt != nil {
			closed = true
		}
	}

	if closed && tier.Type == TierTypeReal {
		go tier.pullHistory(true)
	}

	tier.publishLedger(touched)
}

/*
publishLedger is incremental UI — after one fill or mark only changed lots ship
instead of rebuilding the full tier book every execution row.
*/
func (tier *Tier) publishLedger(positions map[string]*Position) {
	tier.writeLedger(positions, false)
}

/*
publishSnapshot is Hello's full replace — stale chart rows drop when symbols
flat and leave the cache between hellos.
*/
func (tier *Tier) publishSnapshot(positions map[string]*Position) {
	tier.writeLedger(positions, true)
}

/*
writeLedger centralizes UI JSON shape so partial fill updates and Hello snapshots
differ only by positions_snapshot, not duplicate field wiring.
*/
func (tier *Tier) writeLedger(positions map[string]*Position, snapshot bool) {
	if ui.UI == nil || !ui.UI.Active() {
		return
	}

	if positions == nil {
		positions = map[string]*Position{}
	}

	balances := map[string]any{}

	if reading := tier.balanceReading(); reading != nil {
		balances[tier.Type.String()] = reading
	}

	frame := map[string]any{
		"tier":      tier.Type.String(),
		"positions": positions,
		"balances":  balances,
	}

	if snapshot {
		frame["positions_snapshot"] = true
	}

	ui.UI.WriteJSON(frame)
}

/*
fillReport drops venue status chatter — only positive LastQty trade rows may
mutate Qty; otherwise cancels would look like partial sells.
*/
func (tier *Tier) fillReport(execution *market.ExecutionData) bool {
	if execution.LastQty == nil || execution.LastQty.Sign() <= 0 {
		return false
	}

	return execution.ExecType == "" || execution.ExecType == "trade"
}

/*
Hello refreshes marks on open lots and publishes a full positions_snapshot so
balance-driven events show the same PnL kickExit uses, including exchangeClosed.
*/
func (tier *Tier) Hello() {
	positions := make(map[string]*Position)

	for _, position := range tier.positions.All() {
		if position == nil || position.ID == "" {
			continue
		}

		// Closed lots are frozen snapshots — do not re-mark them.
		if position.ClosedAt == nil {
			if tier.price != nil && tier.price.Instrument != nil &&
				!tier.price.Instrument.Has(position.Symbol) {
				positions[position.ID] = position
				continue
			}

			mark := position.Mark

			if tier.price != nil {
				if tier.price.MarkPaused(position.Symbol) {
					if touch := tier.price.CloseMark(position.Symbol); touch != nil {
						mark = touch
					}
				} else if mark == nil {
					mark = tier.price.CloseMark(position.Symbol)
				}
			}

			if mark == nil {
				positions[position.ID] = position
				continue
			}

			_ = position.Update(mark)

			if position.EntryPath != "exchange" {
				tier.kickExit(position)
			}
		}

		positions[position.ID] = position
	}

	if tier.Type == TierTypeReal {
		tier.exchangeClosed.Range(func(_, value any) bool {
			position := value.(*Position)

			if position == nil || position.ID == "" {
				return true
			}

			positions[position.ID] = position

			return true
		})
	}

	tier.publishSnapshot(positions)
}

/*
Update runs on bid marks, not prints, so stoploss advances when the book moves;
exchange lots skip kickExit because exits are manual/history-driven.
*/
func (tier *Tier) Update(data *market.Update) {
	touched := make(map[string]*Position)

	for _, position := range tier.positions.Open(data.Symbol) {
		if err := position.Update(data.Price); err == nil {
			tier.positions.SaveAsync(position)
			touched[position.ID] = position
		}

		if position.EntryPath != "exchange" {
			tier.kickExit(position)
		}
	}

	if len(touched) > 0 {
		tier.publishLedger(touched)
	}
}

/*
Pending retains flat lots after Remove until strategy records exit_path — the lot
is closed in cache but the model still needs the handle for attribution.
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
virgin distinguishes "real never traded" from "real lost paper proof" inside
Unlocked — a bad real track record should not lock out forever on first enable.
*/
func (tier *Tier) virgin() bool {
	return tier.WinRate == 0 && tier.Edge == 0
}

/*
ok is the paper proof signal — disarmed real entries stay forbidden until paper
closed history shows both majority wins and positive average edge.
*/
func (tier *Tier) ok() bool {
	return tier.WinRate > 0.5 && tier.Edge > 0
}

/*
Unlocked: virtual and paper always. Live-only (market.model real) skips the
paper proof — entries are gated upstream and exit on stoploss. Otherwise real
needs paper win-rate above 50% and edge above 0. Once real has history it must
stay there too; after it slips, paper has to lose that proof and earn it again
before real entries resume.
*/
func (tier *Tier) Unlocked(paper *Tier) bool {
	if tier.Type != TierTypeReal {
		tier.unlocked = true
		return true
	}

	if tier.live {
		tier.unlocked = true
		return true
	}

	paperOK := paper != nil && paper.ok()

	if !paperOK {
		tier.unlocked = false
		return false
	}

	if !tier.unlocked {
		if !tier.virgin() && !tier.ok() {
			return tier.unlocked
		}

		tier.unlocked = true
		return true
	}

	if !tier.virgin() && !tier.ok() {
		tier.unlocked = false
		return false
	}

	tier.unlocked = true
	return true
}

/*
Enter binds spot wallet to Allocate so mins and cash match what Send will spend;
transport-less tiers immediately fillsEnter so tests do not need fake ws acks.
*/
func (tier *Tier) Enter(
	symbol string, path string, pathFound bool,
) error {
	if !tier.unlocked {
		return errnie.Error(errnie.Err(
			errnie.Forbidden,
			"[broker.Tier.Enter] tier not unlocked", nil,
		))
	}

	if tier.positions.Has(symbol) {
		return errnie.Err(
			errnie.TooManyRequests,
			"[broker.Tier.Enter] position already exists",
			nil,
		)
	}

	wallet := tier.spotBalance()
	qty, err := tier.price.Allocate(symbol, wallet)

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

	pos := NewPosition(
		tier.ctx,
		symbol,
		tier.Type.String(),
		path,
		tier.price,
		tier.private,
		tier.settle,
	)

	if err := pos.Enter(path, qty, wallet); err != nil {
		return errnie.Error(err)
	}

	tier.positions.Add(symbol, pos, true)

	if err := tier.Send(pos, "enter"); err != nil {
		return errnie.Error(err)
	}

	// Fill Apply owns the first Save/publish. No-transport desks Apply a
	// book fill here so Enter never seeds outside Apply.
	if pos.transport == nil {
		if err := tier.fillsEnter(pos); err != nil {
			tier.positions.Drop(symbol)
			return err
		}
	}

	return nil
}

/*
fillsEnter keeps Apply as the sole fill path for virtual/paper shells — Enter
must not fabricate Qty without walking the same book fill Apply would receive.
*/
func (tier *Tier) fillsEnter(position *Position) error {
	if position == nil || position.EntryOrder == nil {
		return nil
	}

	qty, err := decimal.NewFromString(position.EntryOrder.Volume)
	if err != nil || qty.Sign() <= 0 {
		return errnie.Err(
			errnie.UnprocessableContent,
			"[broker.Tier.fillsEnter] bad entry volume",
			err,
		)
	}

	px := tier.price.FillAsk(position.Symbol, &qty)
	if px == nil || px.Sign() <= 0 {
		return errnie.Err(
			errnie.UnprocessableContent,
			"[broker.Tier.fillsEnter] no book price for "+position.Symbol,
			nil,
		)
	}

	now := time.Now().UTC()
	position.Apply(market.ExecutionData{
		ExecID:        "local-" + position.ID + "-enter",
		ExecType:      "trade",
		ClientOrderID: position.EntryOrder.ClOrdId,
		Symbol:        position.Symbol,
		Side:          "buy",
		LastQty:       &qty,
		LastPrice:     px,
		AvgPrice:      px,
		CumQty:        &qty,
		OrderStatus:   "filled",
		Timestamp:     now,
	})

	if err := tier.positions.Save(position); err != nil {
		return errnie.Error(err)
	}

	tier.publishLedger(map[string]*Position{position.ID: position})
	return nil
}

/*
Exit arms Closing then Send (or fillsExit without transport) so policy exits share
the same sell path as stoploss settle rather than mutating cache timestamps alone.
*/
func (tier *Tier) Exit(
	position *Position,
) error {
	// Policy path arms Closing; exit fills in Apply stamp ClosedAt, then
	// settle Removes. Do not archive before the sell prints.
	position.mu.Lock()
	alreadyClosing := position.Closing
	position.mu.Unlock()

	if err := position.Exit(); err != nil {
		return errnie.Error(err)
	}

	position.mu.Lock()
	newlyArmed := !alreadyClosing && position.Closing
	position.mu.Unlock()

	if !newlyArmed {
		return nil
	}

	if err := tier.Send(position, "exit"); err != nil {
		// Keep Closing: Write may have delivered the sell. Clearing here
		// allows a second Send while the first fill is in flight.
		return errnie.Error(err)
	}

	if tier.private == nil {
		if err := tier.fillsExit(position); err != nil {
			return errnie.Error(err)
		}
	}

	if err := tier.positions.Save(position); err != nil {
		return errnie.Error(err)
	}

	tier.publishLedger(map[string]*Position{position.ID: position})
	return nil
}

/*
fillsExit closes transport-less tiers at MarkClose VWAP so stoploss tests settle
without mocking sell executions on the private socket.
*/
func (tier *Tier) fillsExit(position *Position) error {
	if position == nil {
		return nil
	}

	position.mu.Lock()
	qty := position.Qty
	cl := ""

	if position.ExitOrder != nil {
		cl = position.ExitOrder.ClOrdId
	}

	position.mu.Unlock()

	if qty == nil || qty.Sign() <= 0 {
		return nil
	}

	px := tier.price.CloseMark(position.Symbol)

	if tier.price != nil {
		if vwap, _, _, _ := tier.price.MarkClose(position.Symbol, qty, nil); vwap != nil {
			px = vwap
		}
	}

	if px == nil || px.Sign() <= 0 {
		return errnie.Err(
			errnie.UnprocessableContent,
			"[broker.Tier.fillsExit] no book price for "+position.Symbol,
			nil,
		)
	}

	sellQty := *qty
	now := time.Now().UTC()
	position.Apply(market.ExecutionData{
		ExecID:        "local-" + position.ID + "-exit",
		ExecType:      "trade",
		ClientOrderID: cl,
		Symbol:        position.Symbol,
		Side:          "sell",
		LastQty:       &sellQty,
		LastPrice:     px,
		AvgPrice:      px,
		CumQty:        &sellQty,
		OrderStatus:   "filled",
		Timestamp:     now,
	})

	if position.ClosedAt == nil {
		return errnie.Err(
			errnie.UnprocessableContent,
			"[broker.Tier.fillsExit] lot did not close for "+position.Symbol,
			nil,
		)
	}

	return nil
}

/*
balanceReading exposes cash/unrealized/equity for Hello — on REAL it prefers desk
marks when symm opens exist so the hub matches stoploss PnL, not raw Kraken UV.
*/
func (tier *Tier) balanceReading() map[string]float64 {
	spot := tier.spotBalance()

	if spot == nil {
		return nil
	}

	quote := system.Cfg.Market.QuoteCurrency
	cash := 0.0

	if row := spot.Cash(quote); row != nil {
		cash, _ = row.Float64()
	}

	if tier.Type == TierTypeReal {
		if host, ok := tier.venue.(*venue.Kraken); ok {
			if tb, ok := host.TradeBalance().(*kraken.TradeBalance); ok && tb != nil {
				if eq, un, ok := tier.exchangeBalanceFrom(tb); ok {
					tier.exchange.equity = eq
					tier.exchange.unrealized = un
					tier.exchange.ok = true
				}
			}
		}

		if tier.exchange.ok {
			desk := tier.openUnrealized()
			unrealized := tier.exchange.unrealized

			symmOpen := false

			for _, position := range tier.positions.Open() {
				if position != nil && position.EntryPath != "exchange" &&
					position.Qty != nil && position.Qty.Sign() > 0 {
					symmOpen = true
					break
				}
			}

			if symmOpen {
				unrealized = desk
			} else if unrealized == 0 && desk != 0 {
				unrealized = desk
			}

			return map[string]float64{
				"cash":       cash,
				"unrealized": unrealized,
				"equity":     tier.exchange.equity,
			}
		}

		return map[string]float64{
			"cash":       cash,
			"unrealized": tier.openUnrealized(),
			"equity":     cash,
		}
	}

	unrealized := tier.openUnrealized()

	return map[string]float64{
		"cash":       cash,
		"unrealized": unrealized,
		"equity":     cash + unrealized,
	}
}

/*
exchangeBalanceFrom normalizes Kraken's several equity fields so Hello shows one
consistent cash/unrealized/equity triple on the real tier.
*/
func (tier *Tier) exchangeBalanceFrom(frame market.MarketType) (equity, unrealized float64, ok bool) {
	tb, ok := frame.(*kraken.TradeBalance)

	if !ok || tb == nil {
		return 0, 0, false
	}

	switch {
	case tb.Result.EquivalentBalance != nil && tb.Result.EquivalentBalance.Sign() > 0:
		equity, _ = tb.Result.EquivalentBalance.Float64()
	case tb.Result.Equity != nil && tb.Result.Equity.Sign() > 0:
		equity, _ = tb.Result.Equity.Float64()
	default:
		return 0, 0, false
	}

	unrealized = krakenUnrealized(tb.Result)

	return equity, unrealized, true
}

/*
krakenUnrealized falls back across valuation/cost and equity/trade_balance because
Kraken populates different fields depending on account state and API version.
*/
func krakenUnrealized(result kraken.TradeBalanceResult) float64 {
	if result.UnrealizedPnL != nil && result.UnrealizedPnL.Sign() != 0 {
		f, _ := result.UnrealizedPnL.Float64()
		return f
	}

	if result.Valuation != nil && result.CostBasis != nil {
		diff := result.Valuation.Sub(*result.CostBasis)

		if diff.Sign() != 0 {
			f, _ := diff.Float64()
			return f
		}
	}

	if result.Equity != nil && result.TradeBalance != nil {
		diff := result.Equity.Sub(*result.TradeBalance)

		if diff.Sign() != 0 {
			f, _ := diff.Float64()
			return f
		}
	}

	if result.UnrealizedPnL != nil {
		f, _ := result.UnrealizedPnL.Float64()
		return f
	}

	return 0
}

/*
syncExchangeBalances primes Balance and TradeBalance before Watch's first tick.
*/
func (tier *Tier) syncExchangeBalances() {
	if tier.Type != TierTypeReal || tier.private == nil || tier.venue == nil {
		return
	}

	balanceURL := system.Cfg.WebSocket.Endpoints.Balance
	tradeURL := system.Cfg.WebSocket.Endpoints.TradeBalance
	balance := tier.venue.Balance()
	tradeBalance := tier.venue.TradeBalance()

	if u, ok := balance.(json.Unmarshaler); ok {
		raw, err := tier.private.Post(balanceURL, balance.Request())

		if err != nil {
			errnie.Error(errnie.Err(errnie.IO, "[broker.Tier] balance prime failed", err).With("url", balanceURL))
		} else if len(raw) == 0 {
			errnie.Error(errnie.Err(errnie.IO, "[broker.Tier] balance prime empty", nil).With("url", balanceURL))
		} else if err := u.UnmarshalJSON(raw); err != nil {
			errnie.Error(errnie.Err(errnie.IO, "[broker.Tier] balance prime decode failed", err).With("url", balanceURL))
		}
	}

	if u, ok := tradeBalance.(json.Unmarshaler); ok {
		raw, err := tier.private.Post(tradeURL, tradeBalance.Request())

		if err != nil {
			errnie.Error(errnie.Err(errnie.IO, "[broker.Tier] trade balance prime failed", err).With("url", tradeURL))
		} else if len(raw) == 0 {
			errnie.Error(errnie.Err(errnie.IO, "[broker.Tier] trade balance prime empty", nil).With("url", tradeURL))
		} else if err := u.UnmarshalJSON(raw); err != nil {
			errnie.Error(errnie.Err(errnie.IO, "[broker.Tier] trade balance decode failed", err).With("url", tradeURL))
		}
	}
}

/*
wirePrivateREST primes once, then private Watch keeps venue ledgers fresh.
*/
func (tier *Tier) wirePrivateREST() {
	if tier.private == nil || tier.venue == nil {
		return
	}

	balanceURL := system.Cfg.WebSocket.Endpoints.Balance
	tradeURL := system.Cfg.WebSocket.Endpoints.TradeBalance
	balance := tier.venue.Balance()
	tradeBalance := tier.venue.TradeBalance()

	if tier.Type == TierTypeReal {
		tier.syncExchangeBalances()
	} else if balanceURL != "" && balance != nil {
		if u, ok := balance.(json.Unmarshaler); ok {
			raw, err := tier.private.Post(balanceURL, balance.Request())

			if err != nil {
				errnie.Error(errnie.Err(errnie.IO, "[broker.Tier] balance prime failed", err).With("url", balanceURL))
			} else if len(raw) == 0 {
				errnie.Error(errnie.Err(errnie.IO, "[broker.Tier] balance prime empty", nil).With("url", balanceURL))
			} else if err := u.UnmarshalJSON(raw); err != nil {
				errnie.Error(errnie.Err(errnie.IO, "[broker.Tier] balance prime decode failed", err).With("url", balanceURL))
			}
		}
	}

	if balanceURL != "" && balance != nil {
		tier.private.Watch(balanceURL, balance.Request(), balance)
	}

	if tier.Type == TierTypeReal && tradeURL != "" && tradeBalance != nil {
		tier.private.Watch(tradeURL, tradeBalance.Request(), tradeBalance)
	}
}

/*
openUnrealized replaces exchange unrealized when symm-managed opens exist — venue
unrealized would double-count lots the desk already marks from L3.
*/
func (tier *Tier) openUnrealized() float64 {
	sum := decimal.Zero

	for _, position := range tier.positions.Open() {
		if position == nil {
			continue
		}

		position.mu.Lock()
		pnl := position.PnL
		position.mu.Unlock()

		if pnl != nil {
			sum = sum.Add(*pnl)
		}
	}

	f, _ := sum.Float64()

	return f
}

/*
spotBalance clones venue balance into market.Balance so Enter and Allocate read
the same spot cash the private balance channel just updated.
*/
func (tier *Tier) spotBalance() *market.Balance {
	quote := system.Cfg.Market.QuoteCurrency
	wallet := market.NewBalance(map[string]any{
		"quote":  quote,
		"assets": []*market.Asset{},
	})

	if tier.venue != nil {
		tier.venue.Balance().ToMarket(wallet)
	}

	return wallet
}

/*
venueExitAllowed stops infinite reject loops on cancel-only pairs or sub-min
dust remains after partial sells on REAL.
*/
func (tier *Tier) venueExitAllowed(symbol string, qty *decimal.Decimal) bool {
	if tier == nil || symbol == "" || qty == nil || qty.Sign() <= 0 {
		return false
	}

	if tier.Type != TierTypeReal {
		return true
	}

	if tier.price == nil || tier.price.Instrument == nil {
		return false
	}

	if !tier.price.Instrument.SpotOpen(symbol) {
		return false
	}

	pair := tier.price.Instrument.Pair(symbol)

	if pair.QtyMin != nil && qty.LessThan(*pair.QtyMin) {
		return false
	}

	return true
}

/*
Send is the only wire exit for real orders; token failures drop pending enters
but clear exit cl_ord_id so kickExit can remint after auth heals.
*/
func (tier *Tier) Send(position *Position, action string) error {
	if action == "exit" {
		position.mu.Lock()
		position.ensureExitOrder()
		symbol := position.Symbol
		qty := position.Qty
		position.mu.Unlock()

		if tier.Type == TierTypeReal && !tier.venueExitAllowed(symbol, qty) {
			return errnie.Err(
				errnie.Validation,
				"[broker.Tier.Send] exit blocked for "+symbol+": venue size or market",
				nil,
			)
		}
	}

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

	if tier.Type == TierTypeReal && tier.price != nil && tier.price.Instrument != nil {
		token, err := tier.price.Instrument.WebSocketToken(tier.ctx)

		if err != nil {
			switch action {
			case "exit":
				position.mu.Lock()

				if position.ExitOrder != nil {
					position.ExitOrder.ClOrdId = ""
				}

				position.mu.Unlock()
			case "enter":
				tier.positions.Drop(position.Symbol)
			}

			return errnie.Error(err)
		}

		params["token"] = token
	}

	buf, err := sonic.Marshal(map[string]any{
		"method": "add_order",
		"params": params,
	})

	if err != nil {
		switch action {
		case "exit":
			position.mu.Lock()

			if position.ExitOrder != nil {
				position.ExitOrder.ClOrdId = ""
			}

			position.mu.Unlock()
		case "enter":
			tier.positions.Drop(position.Symbol)
		}

		return errnie.Err(
			errnie.Internal,
			"[broker.Tier.Send] failed to marshal message",
			err,
		)
	}

	if err := tier.private.Write(buf); err != nil {
		if action == "exit" {
			position.mu.Lock()
			position.ExitWireStale = true
			position.mu.Unlock()
		}

		return errnie.Error(err)
	}

	if action == "exit" {
		position.mu.Lock()
		position.ExitWireStale = false
		position.mu.Unlock()
	}

	return nil
}

/*
ackOrder records venue order_id on the lot so fills without cl_ord_id still Owns.
*/
func (tier *Tier) ackOrder(order *kraken.Order) {
	clOrdID := order.Result.ClOrdID
	orderID := order.Result.OrderID

	if clOrdID == "" || orderID == "" {
		return
	}

	for _, position := range tier.positions.Submitted() {
		if position == nil {
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
rejectOrder matches a failed add_order to the lot by cl_ord_id on Entry/ExitOrder.
*/
func (tier *Tier) rejectOrder(order *kraken.Order) {
	clOrdID := order.Result.ClOrdID

	if clOrdID == "" {
		errnie.Error(errnie.Err(
			errnie.NotAcceptable, "[broker.tier] order rejected without cl_ord_id", nil,
		).With("reason", order.Error))
		return
	}

	for _, position := range tier.positions.Submitted() {
		if position == nil {
			continue
		}

		position.mu.Lock()

		switch {
		case position.EntryOrder != nil && position.EntryOrder.ClOrdId == clOrdID:
			filled := position.Qty != nil && position.Qty.Sign() > 0
			position.mu.Unlock()

			if filled {
				errnie.Error(errnie.Err(
					errnie.NotAcceptable, "[broker.tier] entry rejected after fill", nil,
				).With("symbol", position.Symbol, "reason", order.Error))
				tier.Hello()
				return
			}

			tier.positions.Drop(position.Symbol)
			errnie.Error(errnie.Err(
				errnie.NotAcceptable, "[broker.Tier] entry rejected", nil,
			).With("symbol", position.Symbol, "reason", order.Error))
			tier.Hello()
			return

		case position.ExitOrder != nil && position.ExitOrder.ClOrdId == clOrdID:
			position.ExitAttempts++
			attempts := position.ExitAttempts
			position.mu.Unlock()

			errnie.Error(errnie.Err(
				errnie.NotAcceptable, "[broker.tier] exit rejected", nil,
			).With("symbol", position.Symbol, "reason", order.Error, "attempt", attempts))

			const maxExitRejectAttempts = 8

			if attempts >= maxExitRejectAttempts {
				return
			}

			position.mu.Lock()
			position.ExitOrder.ClOrdId = ""
			position.mu.Unlock()

			resend := func() { tier.settle(position) }

			if attempts > 1 {
				delay := time.Duration(attempts-1) * time.Second

				if delay > 15*time.Second {
					delay = 15 * time.Second
				}

				time.AfterFunc(delay, resend)
				return
			}

			resend()
			return
		}

		position.mu.Unlock()
	}
}

/*
kickExit resends a market sell when the lot is closing but the exit never
left (no cl_ord_id) or size drifted after a partial fill.
*/
func (tier *Tier) kickExit(position *Position) {
	if tier == nil || position == nil {
		return
	}

	position.mu.Lock()
	closing := position.Closing && position.ClosedAt == nil
	qtyStr := ""

	if position.Qty != nil && position.Qty.Sign() > 0 {
		qtyStr = position.Qty.String()
	}

	vol, cl := "", ""
	wireStale := position.ExitWireStale

	if position.ExitOrder != nil {
		vol = position.ExitOrder.Volume
		cl = position.ExitOrder.ClOrdId
	}

	position.mu.Unlock()

	if !closing || qtyStr == "" {
		return
	}

	position.mu.Lock()
	qty := position.Qty
	symbol := position.Symbol
	position.mu.Unlock()

	if !tier.venueExitAllowed(symbol, qty) {
		return
	}

	if cl == "" || vol != qtyStr || wireStale {
		tier.settle(position)
	}
}

/*
settle bridges stoploss to Send: flat lots hit pending for the model, closing
lots resend when reject cleared cl_ord_id or partial fill changed qty.
*/
func (tier *Tier) settle(position *Position) {
	if position == nil {
		return
	}

	// Exit fills landed — hand to the model; closed UI comes from TradesHistory.
	if position.ClosedAt != nil {
		tier.finalizeClosedLot(position)
		tier.positions.Remove(position.Symbol)
		tier.Recalculate()
		tier.pending.Store(position.Symbol, position)
		tier.Hello()

		if tier.Type == TierTypeReal {
			go tier.pullHistory(true)
		}

		return
	}

	// Closing armed — send the exit and stay open until Apply flats the lot.
	if err := tier.Send(position, "exit"); err != nil {
		errnie.Error(err)
		return
	}

	if err := tier.positions.Save(position); err != nil {
		errnie.Error(err)
		return
	}

	tier.publishLedger(map[string]*Position{position.ID: position})
}

/*
Recalculate feeds Unlocked — paper must show edge before disarmed real entries
and real must re-earn proof after its own closed history goes negative.
*/
func (tier *Tier) Recalculate() {
	closed := tier.positions.Closed()

	if len(closed) == 0 {
		tier.WinRate = 0
		tier.Edge = 0
		return
	}

	wins := 0
	totalReturn := 0.0
	count := 0

	for _, pos := range closed {
		if pos == nil || pos.ClosedAt == nil || pos.PnLPct == nil {
			continue
		}

		pnlPct, _ := pos.PnLPct.Float64()
		totalReturn += pnlPct

		if pnlPct > 0 {
			wins++
		}

		count++
	}

	if count == 0 {
		tier.WinRate = 0
		tier.Edge = 0
		return
	}

	tier.WinRate = float64(wins) / float64(count)
	tier.Edge = totalReturn / float64(count)
}

/*
OpenPrivate dials the private socket, subscribes executions and balances, primes
REST ledgers, and pulls history once — boot calls this before tier.Run so sends
and fills see auth and snapshots without racing an empty private reader.
*/
func (tier *Tier) OpenPrivate() error {
	if tier.private == nil || tier.venue == nil {
		return nil
	}

	if ws, ok := tier.private.(*network.WebsocketClient); ok && ws != nil {
		if err := ws.Open(system.Cfg.WebSocket.Endpoints.Private); err != nil {
			return errnie.Error(err)
		}

		tier.wirePrivateReconnect(ws)
	}

	if err := tier.resubscribePrivate(); err != nil {
		return errnie.Error(err)
	}

	tier.wirePrivateREST()

	if tier.Type == TierTypeReal {
		tier.purgeSyntheticBalanceLots()
	}

	tier.pullHistory(true)

	return nil
}

/*
wirePrivateReconnect re-attaches execution and balance channels after every
private-socket redial so fills and balances resume without a process restart.
*/
func (tier *Tier) wirePrivateReconnect(ws *network.WebsocketClient) {
	if ws == nil {
		return
	}

	ws.OnReconnect(func() error {
		errnie.Warn("[tier] private stream resubscribe in flight")

		if err := tier.resubscribePrivate(); err != nil {
			return err
		}

		errnie.Info("[tier] private stream recovered")

		return nil
	})
}

func (tier *Tier) resubscribePrivate() error {
	wire := tier.venue.Execution()

	if wire == nil {
		return nil
	}

	req := wire.Request()

	if tier.Type == TierTypeReal && tier.price != nil && tier.price.Instrument != nil {
		token, err := tier.price.Instrument.WebSocketToken(tier.ctx)

		if err != nil {
			return errnie.Error(err)
		}

		params, ok := req["params"].(map[string]any)

		if ok {
			params["token"] = token
		}
	}

	buf, err := sonic.Marshal(req)

	if err != nil {
		return errnie.Error(err)
	}

	if err := tier.private.Write(buf); err != nil {
		return err
	}

	if err := tier.subscribePrivate("balances"); err != nil {
		return err
	}

	return nil
}

/*
subscribePrivate shares the ws token path with add_order so balance and
execution channels auth the same way on REAL.
*/
func (tier *Tier) subscribePrivate(channel string) error {
	if tier.private == nil || tier.venue == nil {
		return nil
	}

	params := map[string]any{
		"channel":  channel,
		"snapshot": true,
	}

	if tier.Type == TierTypeReal && tier.price != nil && tier.price.Instrument != nil {
		token, err := tier.price.Instrument.WebSocketToken(tier.ctx)

		if err != nil {
			return errnie.Error(err)
		}

		params["token"] = token
	}

	buf, err := sonic.Marshal(map[string]any{
		"method": "subscribe",
		"params": params,
	})

	if err != nil {
		return errnie.Error(err)
	}

	return tier.private.Write(buf)
}

/*
Run replays settle for lots restored mid-close after restart so a process bounce
does not strand Closing lots without a private reader attached.
*/
func (tier *Tier) Run() {
	if tier.private == nil || tier.venue == nil {
		return
	}

	// Resend exits for lots restored mid-close once private ingress is up.
	for _, position := range tier.positions.Open() {
		if position != nil && position.Closing && position.ClosedAt == nil {
			tier.settle(position)
		}
	}

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

/*
AssembledTier skips NewTier's venue hooks so model tests own mocks without
balance Watch firing history or fill callbacks during table tests.
*/
func AssembledTier(
	ctx context.Context,
	tt TierType,
	price *Price,
	host market.Venue,
	transport network.Client,
) *Tier {
	return &Tier{
		ctx:       ctx,
		Type:      tt,
		unlocked:  true,
		price:     price,
		venue:     host,
		private:   transport,
		positions: NewCache(ctx, tt.String(), transport, price, nil, nil),
		pending:   &sync.Map{},
	}
}
