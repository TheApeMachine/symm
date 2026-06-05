package trader

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/focus"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/market/perspectives"
)

const (
	cryptoRawSubscriberID    = "trader/crypto:raw"
	cryptoWalletSubscriberID = "trader/crypto:wallet"
)

/*
Crypto publishes wallet snapshots to the ui broadcast from Kraken balance frames.
*/
// armRecord is the protective trigger currently resting on the exchange for a
// symbol, so the trader places it once and does not re-submit an identical one each
// tick (the exchange holds it). A changed type/offset re-arms.
type armRecord struct {
	action perspectives.ActionType
	offset float64
}

type Crypto struct {
	ctx            context.Context
	cancel         context.CancelFunc
	pool           *qpool.Q
	ui             *qpool.BroadcastGroup
	broadcasts     map[string]*qpool.BroadcastGroup
	subscribers    map[string]*qpool.Subscriber
	desk           *broker.Desk
	quotes         *broker.QuoteCache
	streams        *focus.Set
	audit          *audit.Writer
	pendingOrders  sync.Map
	auditSeq       atomic.Int64
	balanceOnce    sync.Once
	inventory      map[string]float64
	avgEntry       map[string]float64
	armed          map[string]armRecord
	pending        map[string]perspectives.Action
	lastDecision   map[string]string
	walletCurrency string
	capitalBase    float64 // opening capital; one position targets position_fraction of this
	walletMu       sync.RWMutex
	availableQuote float64 // latest wallet balance the API (balances.go) published
}

// minSlotFillFraction is the smallest share of a target position slot an entry may
// fund and still be placed. Below it the entry is rejected rather than deployed,
// which caps concurrent positions at ~1/position_fraction and stops a fully
// deployed wallet from cascading its leftover dust into ever-smaller positions.
const minSlotFillFraction = 0.5

// markInterval is how often the trader publishes the live mark price of each held
// symbol, so the dashboard shows real-time unrealized P&L on open positions.
const markInterval = 500 * time.Millisecond

func NewCrypto(ctx context.Context, pool *qpool.Q, streams *focus.Set) *Crypto {
	return NewCryptoWithCaches(
		ctx,
		pool,
		streams,
		broker.EnsureQuoteCache(ctx, pool),
		broker.EnsureStressCache(ctx, pool),
	)
}

/*
NewCryptoWithCaches builds the trader against explicit broker state caches.
*/
func NewCryptoWithCaches(
	ctx context.Context,
	pool *qpool.Q,
	streams *focus.Set,
	quotes *broker.QuoteCache,
	stress *broker.StressCache,
) *Crypto {
	ctx, cancel := context.WithCancel(ctx)

	desk, err := broker.NewDeskWithCaches(ctx, pool, quotes, stress)

	if err != nil {
		cancel()
		errnie.Error(fmt.Errorf("trader/crypto: desk: %w", err), "trader/crypto")
		return nil
	}

	auditWriter, err := audit.OpenWriter()

	if err != nil {
		cancel()
		errnie.Error(fmt.Errorf("trader/crypto: audit: %w", err), "trader/crypto")
		return nil
	}

	crypto := &Crypto{
		ctx:          ctx,
		cancel:       cancel,
		pool:         pool,
		ui:           pool.CreateBroadcastGroup("ui", 10*time.Millisecond),
		broadcasts:   make(map[string]*qpool.BroadcastGroup),
		subscribers:  make(map[string]*qpool.Subscriber),
		streams:      streams,
		desk:         desk,
		quotes:       quotes,
		audit:        auditWriter,
		inventory:    make(map[string]float64),
		avgEntry:     make(map[string]float64),
		armed:        make(map[string]armRecord),
		pending:      make(map[string]perspectives.Action),
		lastDecision: make(map[string]string),
	}

	for _, channel := range []string{"raw"} {
		crypto.broadcasts[channel] = pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
		crypto.subscribers[channel] = crypto.broadcasts[channel].Subscribe(cryptoRawSubscriberID, 1024)
	}

	crypto.walletCurrency = strings.ToUpper(viper.GetString("market.quote_currency"))
	// Seed with the configured opening balance balances.go starts from; every
	// subsequent value is read from the wallet snapshots it publishes — crypto
	// never recomputes its own balance.
	crypto.availableQuote = viper.GetFloat64(
		"trading.paper.wallet_" + strings.ToLower(crypto.walletCurrency),
	)
	// The opening balance is the capital base each position is sized against, so a
	// full-deployment fraction means one position at a time, not a cascade that
	// empties the wallet into the first symbol and dusts the rest.
	crypto.capitalBase = crypto.availableQuote
	crypto.subscribers["ui"] = crypto.ui.Subscribe(cryptoWalletSubscriberID, 256)

	go crypto.watchWallet()

	errnie.Info("trader/crypto ready", "trader/crypto")
	return crypto
}

/*
Tick consumes the raw bus. Story emits Action structs (entry/exit verdicts) and
the paper desk emits execution maps. Actions submit orders; executions update
inventory and the shared focus set so the not-holding gate sees open positions.
*/
func (crypto *Crypto) Tick() error {
	incoming := crypto.subscribers["raw"].Incoming
	marks := time.NewTicker(markInterval)
	defer marks.Stop()

	for {
		select {
		case <-crypto.ctx.Done():
			return crypto.ctx.Err()
		case <-marks.C:
			// Same goroutine that mutates inventory, so the read is race-free.
			crypto.publishMarks()
		case message := <-incoming:
			if message == nil || message.Value == nil {
				continue
			}

			if action, ok := message.Value.(perspectives.Action); ok {
				crypto.submit(action)
				continue
			}

			if envelope, ok := message.Value.(map[string]any); ok {
				crypto.observeExecution(envelope)
			}
		}
	}
}

/*
submit forwards an entry or exit verdict to the desk. One in-flight order per
symbol is allowed: entries only when flat, exits only when holding, and never
while an order for that symbol is still resolving. This is race-free because
Tick processes actions and executions on a single goroutine. Exit verdicts carry
no quantity, so we settle the full position we currently hold.
*/
func (crypto *Crypto) submit(action perspectives.Action) {
	if action.Type == perspectives.ActionNone {
		return
	}

	if _, inFlight := crypto.pending[action.Symbol]; inFlight {
		crypto.publishDecision(action, "rejected", "order still resolving")
		return
	}

	held := crypto.inventory[action.Symbol]

	if perspectives.IsEntryAction(action.Type) {
		if held > 0 {
			crypto.publishDecision(action, "rejected", "already holding")
			return
		}

		if !crypto.fundableSymbol(action.Symbol) {
			crypto.publishDecision(
				action, "rejected", "wallet holds no "+quoteCurrency(action.Symbol),
			)
			return
		}

		// Size the order against the wallet balance the API published, never a
		// locally tracked figure. balances.ApplyFill remains the final gate.
		quantity := crypto.sizeEntry(action.Price)

		if quantity <= 0 {
			crypto.publishDecision(action, "rejected", "insufficient funds")
			return
		}

		action.Quantity = quantity
	}

	if perspectives.IsExitAction(action.Type) {
		if held <= 0 {
			crypto.publishDecision(action, "rejected", "not holding")
			return
		}

		action.Quantity = held
	}

	protective := perspectives.IsProtectiveExit(action.Type)

	if protective {
		// Stop/take levels are measured from the entry price the position holds.
		// Refuse to arm at a zero level if the entry price is somehow unknown,
		// rather than rest a trigger that could never fire.
		entry := crypto.avgEntry[action.Symbol]

		if entry <= 0 {
			crypto.publishDecision(action, "rejected", "no entry price to anchor the protective trigger")
			return
		}

		action.Price = entry

		resolved, err := crypto.desk.ResolveAction(action)

		if err != nil {
			crypto.publishDecision(resolved, "rejected", cleanReason(err.Error()))
			return
		}

		action = resolved

		// The same protective trigger fires every tick the management gate holds;
		// the exchange already rests one, so only (re)place it when it changes.
		if crypto.armed[action.Symbol] == (armRecord{action.Type, action.Offset}) {
			return
		}
	}

	// A desk rejection is the gate working as intended, not an in-flight order.
	accepted, err := crypto.desk.AddOrder(action)

	if err != nil {
		action = accepted
		crypto.publishDecision(action, "rejected", cleanReason(err.Error()))
		return
	}

	action = accepted
	crypto.publishDecision(action, "submitted", "")

	// A protective trigger rests on the exchange — it has no immediate fill, so it
	// does not block the symbol; record it so identical re-arms are skipped.
	if protective {
		crypto.armed[action.Symbol] = armRecord{action.Type, action.Offset}
		return
	}

	crypto.pending[action.Symbol] = action
}

/*
publishDecision emits one decision card to the dashboard explaining what happened
to a verdict — submitted, filled, or rejected with the precise reason. It dedupes
per symbol so a signal that fires every tick against the same gate does not flood
the panel; only a change of verdict or reason re-emits.
*/
func (crypto *Crypto) publishDecision(
	action perspectives.Action, verdict, reason string,
) {
	key := verdict + "|" + reason

	if crypto.lastDecision[action.Symbol] == key {
		return
	}

	crypto.lastDecision[action.Symbol] = key

	if err := crypto.writeDecisionAudit(action, verdict, reason); err != nil {
		errnie.Error(err)
	}

	if crypto.ui == nil {
		return
	}

	crypto.ui.Send(&qpool.QValue[any]{Value: map[string]any{
		"event":   "decision",
		"type":    action.Type.String(),
		"symbol":  action.Symbol,
		"side":    string(action.Side),
		"verdict": verdict,
		"reason":  reason,
	}})
}

func (crypto *Crypto) writeDecisionAudit(
	action perspectives.Action,
	verdict string,
	reason string,
) error {
	if crypto.audit == nil {
		return nil
	}

	return crypto.audit.Write(map[string]any{
		"audit_event":  "trade_decision",
		"symbol":       action.Symbol,
		"type":         action.Type.String(),
		"side":         string(action.Side),
		"verdict":      verdict,
		"block_reason": reason,
		"price":        action.Price,
		"quantity":     action.Quantity,
		"offset":       action.Offset,
	})
}

// cleanReason strips the internal prefixes and trailing "for SYMBOL" suffix from
// a gate/fill error so the dashboard shows a tight human reason (the card already
// carries the symbol separately).
func cleanReason(reason string) string {
	for _, prefix := range []string{"preflight: ", "paper fill: ", "paper balances: "} {
		reason = strings.TrimPrefix(reason, prefix)
	}

	if index := strings.Index(reason, " for "); index >= 0 {
		reason = reason[:index]
	}

	return reason
}

/*
watchWallet keeps availableQuote in step with the wallet balance balances.go
publishes on the ui bus. It only ever reflects the API's authoritative figure —
crypto never derives a balance of its own — so there is a single source of truth.
*/
func (crypto *Crypto) watchWallet() {
	incoming := crypto.subscribers["ui"].Incoming

	for {
		select {
		case <-crypto.ctx.Done():
			return
		case message := <-incoming:
			if message == nil {
				continue
			}

			frame, ok := message.Value.(map[string]any)

			if !ok || frame["event"] != "wallet" {
				continue
			}

			balance, _ := frame["balance"].(float64)

			crypto.walletMu.Lock()
			crypto.availableQuote = balance
			// Seed the capital base from the first real balance when no opening
			// balance was configured (live), so the per-position slot cap still
			// applies instead of falling back to deploying all available cash.
			if crypto.capitalBase <= 0 && balance > 0 {
				crypto.capitalBase = balance
			}
			crypto.walletMu.Unlock()
		}
	}
}

func (crypto *Crypto) availableCash() float64 {
	crypto.walletMu.RLock()
	defer crypto.walletMu.RUnlock()

	return crypto.availableQuote
}

func (crypto *Crypto) capitalBaseValue() float64 {
	crypto.walletMu.RLock()
	defer crypto.walletMu.RUnlock()

	return crypto.capitalBase
}

// fundableSymbol reports whether the wallet can pay for the pair: a buy spends the
// pair's quote currency, so only pairs quoted in walletCurrency are tradeable.
func (crypto *Crypto) fundableSymbol(symbol string) bool {
	if crypto.walletCurrency == "" {
		return true
	}

	return quoteCurrency(symbol) == crypto.walletCurrency
}

/*
sizeEntry sizes a buy as one position slot — position_fraction of the capital base
— funded from the published wallet balance, leaving room for the entry fee so the
order clears ApplyFill. An entry that cannot fund at least minSlotFillFraction of a
slot returns 0 (rejected), so position_fraction caps concurrent positions at
~1/fraction (1.0 = one at a time) instead of deploying 100% of whatever cash is
left on every symbol — the cascade that drained the wallet into dust.
*/
func (crypto *Crypto) sizeEntry(price float64) float64 {
	if price <= 0 {
		return 0
	}

	fraction := viper.GetFloat64("trading.position_fraction")

	if fraction <= 0 || fraction > 1 {
		fraction = 1.0
	}

	makerFee := viper.GetFloat64("trading.paper.maker_fee_pct") / 100
	cash := crypto.availableCash()

	// A position targets a fixed slot of the capital base. Fall back to available
	// cash only when no capital base is known yet (no opening balance, no wallet
	// frame seen).
	slot := fraction * crypto.capitalBaseValue()

	if slot <= 0 {
		slot = cash
	}

	// Spend one slot, but never more than the wallet can fund with fee headroom.
	spend := slot

	if affordable := cash / (1 + makerFee); spend > affordable {
		spend = affordable
	}

	// Reject entries that cannot fund a meaningful share of a slot. This is the gate
	// that enforces the concurrent-position cap and kills the dust cascade.
	if spend <= 0 || spend < slot*minSlotFillFraction {
		return 0
	}

	return spend / (price * (1 + makerFee))
}

func quoteCurrency(symbol string) string {
	if slash := strings.LastIndex(symbol, "/"); slash >= 0 {
		return symbol[slash+1:]
	}

	return symbol
}

/*
observeExecution applies a paper fill to inventory and the focus set, clearing
the in-flight marker. A buy opens or adds to a position; a settling sell closes
it. A zero-quantity execution is a no-fill: it only clears the in-flight marker.
*/
func (crypto *Crypto) observeExecution(envelope map[string]any) {
	if envelope["channel"] != "executions" {
		return
	}

	symbol, _ := envelope["symbol"].(string)

	if symbol == "" {
		return
	}

	submitted := crypto.pending[symbol]
	submitted.Symbol = symbol
	delete(crypto.pending, symbol)

	side, _ := envelope["side"].(string)
	qty, _ := envelope["qty"].(float64)
	price, _ := envelope["price"].(float64)

	if qty <= 0 {
		reason, _ := envelope["reason"].(string)

		if reason == "" {
			reason = "no fill"
		}

		crypto.publishDecision(submitted, "rejected", cleanReason(reason))
		return
	}

	crypto.publishDecision(submitted, "filled", "")

	if side == string(trading.Buy) {
		crypto.openPosition(symbol, qty, price)
		return
	}

	crypto.closePosition(symbol)
}

func (crypto *Crypto) openPosition(symbol string, qty, price float64) {
	prevQty := crypto.inventory[symbol]
	prevCost := prevQty * crypto.avgEntry[symbol]
	newQty := prevQty + qty

	crypto.inventory[symbol] = newQty
	crypto.avgEntry[symbol] = (prevCost + qty*price) / newQty
	crypto.streams.Add(symbol)
	crypto.publishPositions()
}

func (crypto *Crypto) closePosition(symbol string) {
	delete(crypto.inventory, symbol)
	delete(crypto.avgEntry, symbol)
	delete(crypto.armed, symbol)
	crypto.streams.Remove(symbol)
	crypto.publishPositions()
}

/*
publishPositions ships the open book to the dashboard so it can mark each
position against the live price stream and show real-time unrealized P&L.
*/
func (crypto *Crypto) publishPositions() {
	if crypto.ui == nil {
		return
	}

	positions := make([]map[string]any, 0, len(crypto.inventory))

	for symbol, qty := range crypto.inventory {
		if qty <= 0 {
			continue
		}

		positions = append(positions, map[string]any{
			"symbol":    symbol,
			"qty":       qty,
			"avg_entry": crypto.avgEntry[symbol],
		})
	}

	crypto.ui.Send(&qpool.QValue[any]{Value: map[string]any{
		"event":     "positions",
		"positions": positions,
	}})
}

/*
publishMarks streams the live mark price of each held symbol to the dashboard so the
open-positions panel can show real-time unrealized P&L. Positions are published only
when they open or close, so without a continuous mark the panel marks every position
at its own entry and reads a flat €0.00. Runs on the Tick goroutine, so the inventory
read is race-free.
*/
func (crypto *Crypto) publishMarks() {
	if crypto.ui == nil || crypto.quotes == nil || len(crypto.inventory) == 0 {
		return
	}

	for symbol := range crypto.inventory {
		quote, ok := crypto.quotes.Snapshot(symbol)

		if !ok {
			continue
		}

		price := markPrice(quote)

		if price <= 0 {
			continue
		}

		crypto.ui.Send(&qpool.QValue[any]{Value: map[string]any{
			"event":  "mark",
			"symbol": symbol,
			"price":  price,
		}})
	}
}

// markPrice values an open long at the price it could actually be sold into right
// now — the bid. Entries are buys that fill at the ask (crossing the spread up), and
// an exit (settle_position) sells into the bid, so marking at the bid makes a fresh
// position open down the round-trip spread (realistic) rather than showing a phantom
// profit from a higher last/mid reference. Falls back to the last trade, then the
// ask, only when no bid is available.
func markPrice(quote broker.Quote) float64 {
	if quote.Bid > 0 {
		return quote.Bid
	}

	if quote.Last > 0 {
		return quote.Last
	}

	return quote.Ask
}

func (crypto *Crypto) Close() error {
	if crypto.cancel != nil {
		crypto.cancel()
	}

	if crypto.audit == nil {
		return nil
	}

	closeErr := crypto.audit.Close()
	crypto.audit = nil

	return closeErr
}
