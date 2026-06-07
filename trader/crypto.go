package trader

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/bus"
	"github.com/theapemachine/symm/focus"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/market/perspectives/reasoning"
)

const (
	cryptoRawSubscriberID    = "trader/crypto:raw"
	cryptoWalletSubscriberID = "trader/crypto:wallet"
	// symbolSubmissionCooldown isolates a rejected symbol without affecting the rest
	// of the portfolio or halting the desk.
	symbolSubmissionCooldown = 10 * time.Second
)

/*
Crypto publishes wallet snapshots to the ui broadcast from Kraken balance frames.
*/
// armRecord is the protective trigger currently resting on the exchange for a
// symbol, so the trader places it once and does not re-submit an identical one each
// tick (the exchange holds it). A changed type/offset re-arms.
type armRecord struct {
	action reasoning.ActionType
	offset float64
}

type Crypto struct {
	ctx              context.Context
	cancel           context.CancelFunc
	pool             *qpool.Q[any]
	ui               *qpool.BroadcastGroup
	broadcasts       map[string]*qpool.BroadcastGroup
	subscribers      map[string]*qpool.BroadcastConsumer
	desk             *broker.Desk
	quotes           *broker.QuoteCache
	streams          *focus.Set
	audit            *audit.Writer
	pendingOrders    sync.Map
	auditSeq         atomic.Int64
	balanceOnce      sync.Once
	inventory        map[string]float64
	shortInventory   map[string]float64
	avgEntry         map[string]float64
	armed            map[string]armRecord
	pending          map[string]reasoning.Action
	coolDownUntil    sync.Map // symbol -> time.Time (per-symbol submission backoff)
	lastDecision     map[string]string
	entryBatch       entryBatch
	preemptPlan      *preemptPlan
	entryConviction  map[string]float64
	walletCurrency   string
	positionFraction float64 // validated share of capital per position; in (0, 1]
	capitalBase      float64 // opening capital; one position targets position_fraction of this
	walletMu         sync.RWMutex
	heldSnapshot     atomic.Pointer[map[string]struct{}]
	availableQuote   float64 // latest wallet balance the API (balances.go) published
}

// markInterval is how often the trader publishes the live mark price of each held
// symbol, so the dashboard shows real-time unrealized P&L on open positions.
const markInterval = 500 * time.Millisecond

func NewCrypto(ctx context.Context, pool *qpool.Q[any], streams *focus.Set) *Crypto {
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
	pool *qpool.Q[any],
	streams *focus.Set,
	quotes *broker.QuoteCache,
	stress *broker.StressCache,
) *Crypto {
	ctx, cancel := context.WithCancel(ctx)

	fraction := viper.GetFloat64("trading.position_fraction")

	if fraction <= 0 || fraction > 1 {
		cancel()
		errnie.Error(fmt.Errorf(
			"trader/crypto: trading.position_fraction must be in (0, 1], got %v", fraction,
		), "trader/crypto")
		return nil
	}

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
		ctx:             ctx,
		cancel:          cancel,
		pool:            pool,
		ui:              bus.Group(pool, "ui", 10*time.Millisecond),
		broadcasts:      make(map[string]*qpool.BroadcastGroup),
		subscribers:     make(map[string]*qpool.BroadcastConsumer),
		streams:         streams,
		desk:            desk,
		quotes:          quotes,
		audit:           auditWriter,
		inventory:       make(map[string]float64),
		shortInventory:  make(map[string]float64),
		avgEntry:        make(map[string]float64),
		armed:           make(map[string]armRecord),
		pending:         make(map[string]reasoning.Action),
		lastDecision:    make(map[string]string),
		entryConviction: make(map[string]float64),
	}

	for _, channel := range []string{"raw"} {
		crypto.broadcasts[channel] = bus.Group(pool, channel, 10*time.Millisecond)
		crypto.subscribers[channel] = crypto.broadcasts[channel].Subscribe(cryptoRawSubscriberID, 1024)
	}

	crypto.positionFraction = fraction
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
	crypto.syncHeldSnapshot()

	go crypto.watchWallet()

	errnie.Info("trader/crypto ready", "trader/crypto")
	return crypto
}

/*
SymbolHeld reports whether the trader currently holds a long or short position in
symbol according to the exchange-reconciled inventory (holdings snapshots and
fills). Story uses this for playbook lifecycle predicates; the chart focus set is
UI-only and is not consulted here.
*/
func (crypto *Crypto) SymbolHeld(symbol string) bool {
	snapshot := crypto.heldSnapshot.Load()

	if snapshot == nil {
		return false
	}

	_, held := (*snapshot)[symbol]

	return held
}

func (crypto *Crypto) syncHeldSnapshot() {
	next := make(map[string]struct{})

	for symbol, qty := range crypto.inventory {
		if qty > 0 {
			next[symbol] = struct{}{}
		}
	}

	for symbol, qty := range crypto.shortInventory {
		if qty > 0 {
			next[symbol] = struct{}{}
		}
	}

	crypto.heldSnapshot.Store(&next)
}

/*
Tick consumes the raw bus. Story emits Action structs (entry/exit verdicts) and
the paper desk emits execution maps. Actions submit orders; executions update
inventory and the shared focus set so the not-holding gate sees open positions.
*/
func (crypto *Crypto) Tick() error {
	raw := crypto.subscribers["raw"]
	marks := time.NewTicker(markInterval)
	defer marks.Stop()

	for {
		select {
		case <-crypto.ctx.Done():
			return crypto.ctx.Err()
		case <-marks.C:
			crypto.publishMarks()
			crypto.publishEquity()
			if crypto.entryBatchDue(time.Now()) {
				crypto.flushEntryBatch()
			}
		default:
			message := raw.Poll()

			if message == nil {
				select {
				case <-crypto.ctx.Done():
					return crypto.ctx.Err()
				case <-marks.C:
					crypto.publishMarks()
					crypto.publishEquity()
					if crypto.entryBatchDue(time.Now()) {
						crypto.flushEntryBatch()
					}
				case <-time.After(2 * time.Millisecond):
				}

				continue
			}

			if message.Value == nil {
				continue
			}

			if action, ok := message.Value.(reasoning.Action); ok {
				crypto.routeAction(action)

				if crypto.entryBatchDue(time.Now()) {
					crypto.flushEntryBatch()
				}

				continue
			}

			if envelope, ok := message.Value.(map[string]any); ok {
				crypto.observeExecution(envelope)
				crypto.observeBalances(envelope)
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
func (crypto *Crypto) submit(action reasoning.Action) {
	if action.Type == reasoning.ActionNone {
		return
	}

	if crypto.symbolCooling(action.Symbol) {
		return
	}

	if _, inFlight := crypto.pending[action.Symbol]; inFlight {
		crypto.publishDecision(action, "rejected", "order still resolving")
		return
	}

	held := crypto.inventory[action.Symbol]

	if reasoning.IsEntryAction(action.Type) {
		if action.Side == trading.Sell && !viper.GetBool("trading.margin_enabled") {
			crypto.publishDecision(action, "rejected", "short entries disabled")
			return
		}

		if held > 0 {
			crypto.publishDecision(action, "rejected", "already holding long")
			return
		}

		if crypto.shortInventory[action.Symbol] > 0 {
			crypto.publishDecision(action, "rejected", "already holding short")
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
		quantity, err := crypto.sizeEntry(action)

		if err != nil {
			crypto.publishDecision(action, "rejected", err.Error())
			return
		}

		if quantity <= 0 {
			crypto.publishDecision(action, "rejected", "insufficient funds")
			return
		}

		action.Quantity = quantity
	}

	if reasoning.IsExitAction(action.Type) {
		if crypto.shortInventory[action.Symbol] > 0 {
			action.Side = trading.Buy
			action.Quantity = crypto.shortInventory[action.Symbol]
		} else if held > 0 {
			action.Side = trading.Sell
			action.Quantity = held
		} else {
			crypto.publishDecision(action, "rejected", "not holding")
			return
		}
	}

	protective := reasoning.IsProtectiveExit(action.Type)

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
			if isTransientQuoteMiss(err) {
				return
			}

			crypto.publishDecision(resolved, "rejected", cleanReason(err.Error()))
			return
		}

		action = resolved

		// The same protective trigger fires every tick the management gate holds;
		// the exchange already rests one, so only (re)place it when it changes.
		armKey := action.Symbol + ":" + string(action.Side)
		if crypto.armed[armKey] == (armRecord{action.Type, action.Offset}) {
			return
		}
	}

	// A desk rejection is the gate working as intended, not an in-flight order.
	accepted, err := crypto.desk.AddOrder(action)

	if err != nil {
		action = accepted

		if isTransientQuoteMiss(err) {
			return
		}

		crypto.publishDecision(action, "rejected", cleanReason(err.Error()))
		crypto.coolSymbol(action.Symbol)

		return
	}

	action = accepted
	crypto.publishDecision(action, "submitted", "")

	// A protective trigger rests on the exchange — it has no immediate fill, so it
	// does not block the symbol; record it so identical re-arms are skipped.
	if protective {
		armKey := action.Symbol + ":" + string(action.Side)
		crypto.armed[armKey] = armRecord{action.Type, action.Offset}
		return
	}

	crypto.pending[action.Symbol] = action
}

func (crypto *Crypto) symbolCooling(symbol string) bool {
	untilRaw, cooling := crypto.coolDownUntil.Load(symbol)

	if !cooling {
		return false
	}

	until, ok := untilRaw.(time.Time)

	if !ok {
		crypto.coolDownUntil.Delete(symbol)

		return false
	}

	if time.Now().Before(until) {
		return true
	}

	crypto.coolDownUntil.Delete(symbol)

	return false
}

func (crypto *Crypto) coolSymbol(symbol string) {
	crypto.coolDownUntil.Store(symbol, time.Now().Add(symbolSubmissionCooldown))
}

/*
publishDecision emits one decision card to the dashboard explaining what happened
to a verdict — submitted, filled, or rejected with the precise reason. It dedupes
per symbol so a signal that fires every tick against the same gate does not flood
the panel; only a change of verdict or reason re-emits.
*/
func (crypto *Crypto) publishDecision(
	action reasoning.Action, verdict, reason string,
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
	action reasoning.Action,
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
		"fraction":     action.Fraction,
		"regime":       action.Regime.String(),
	})
}

// cleanReason strips the internal prefixes and trailing "for SYMBOL" suffix from
// a gate/fill error so the dashboard shows a tight human reason (the card already
// carries the symbol separately).
func isTransientQuoteMiss(err error) bool {
	if err == nil {
		return false
	}

	return strings.Contains(err.Error(), "no quote for")
}

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
	ui := crypto.subscribers["ui"]

	for {
		message, err := ui.Wait(crypto.ctx)

		if err != nil {
			return
		}

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
		if crypto.capitalBase <= 0 && balance > 0 {
			crypto.capitalBase = balance
		}
		crypto.walletMu.Unlock()
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
sizeEntry sizes a buy as one position slot — position_fraction of the capital base.
position_fraction is the share of capital committed per position, so the desk holds
at most floor(1/fraction) positions at once: once that many are committed (held plus
in flight) a new entry is refused, instead of splitting the remaining cash into
ever-smaller positions. A position cannot be sized against capital we do not yet know,
so it returns 0 until a capital base exists rather than substituting the live cash.
*/
func (crypto *Crypto) sizeEntry(action reasoning.Action) (float64, error) {
	price := action.Price

	if price <= 0 {
		return 0, fmt.Errorf("trader: entry price must be positive")
	}

	capital := crypto.capitalBaseValue()

	if capital <= 0 {
		return 0, nil
	}

	fraction, err := crypto.entryDeployFraction(action)

	if err != nil {
		return 0, err
	}

	if fraction <= 0 {
		return 0, nil
	}

	if fraction > 1 {
		return 0, fmt.Errorf("trader: deploy fraction %.4f exceeds 1", fraction)
	}

	// At most floor(1/fraction) positions run at once. A 60% slot must not permit
	// two simultaneous positions; cash would shrink the second, silently changing the
	// strategy the optimizer scored. Pending entry orders count too.
	capacity := int(math.Floor(1/fraction + 1e-9))

	if capacity < 1 {
		return 0, fmt.Errorf("trader: deploy fraction %.4f exceeds concurrent capacity", fraction)
	}

	if crypto.openExposureCount() >= capacity {
		return 0, nil
	}

	feeRate := crypto.entryFeeRate(action.Type)

	// One position deploys the selected fraction of the capital base in total
	// spend. Convert that total spend to fee-aware notional, then bound it by what
	// the wallet can fund right now. This keeps two 50% slots from exceeding the
	// wallet by the entry fees.
	slot := fraction * capital / (1 + feeRate)

	if affordable := crypto.availableCash() / (1 + feeRate); slot > affordable {
		slot = affordable
	}

	if slot <= 0 {
		return 0, nil
	}

	return slot / price, nil
}

func (crypto *Crypto) entryFeeRate(actionType reasoning.ActionType) float64 {
	if reasoning.IsMakerAction(actionType) {
		return broker.MakerFeePctFromViper() / 100
	}

	return broker.TakerFeePctFromViper() / 100
}

func (crypto *Crypto) entryDeployFraction(action reasoning.Action) (float64, error) {
	fraction := crypto.positionFraction

	if action.Fraction > 0 {
		fraction *= action.Fraction
	}

	scale, err := perspectives.RegimeSizeScale(action.Regime)

	if err != nil {
		return 0, err
	}

	fraction *= scale

	if fraction < 0 {
		return 0, nil
	}

	return fraction, nil
}

func (crypto *Crypto) openExposureCount() int {
	count := len(crypto.inventory) + len(crypto.shortInventory)

	for _, pending := range crypto.pending {
		if reasoning.IsEntryAction(pending.Type) {
			count++
		}
	}

	return count
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
		crypto.coolSymbol(symbol)

		return
	}

	crypto.publishDecision(submitted, "filled", "")

	if submitted.Type == reasoning.ActionNone {
		if side == string(trading.Buy) {
			crypto.openPosition(symbol, qty, price)
		} else {
			crypto.closePosition(symbol)
		}

		return
	}

	if side == string(trading.Buy) {
		if reasoning.IsEntryAction(submitted.Type) {
			crypto.recordEntryConviction(symbol, submitted)
			crypto.openPosition(symbol, qty, price)
		} else {
			crypto.closeShort(symbol)
			crypto.tryFinishPreemption(symbol)
		}

		return
	}

	if reasoning.IsEntryAction(submitted.Type) {
		crypto.recordEntryConviction(symbol, submitted)
		crypto.openShort(symbol, qty, price)

		return
	}

	crypto.closePosition(symbol)
	crypto.tryFinishPreemption(symbol)
}

func (crypto *Crypto) openPosition(symbol string, qty, price float64) {
	prevQty := crypto.inventory[symbol]
	prevCost := prevQty * crypto.avgEntry[symbol]
	newQty := prevQty + qty

	crypto.inventory[symbol] = newQty
	crypto.avgEntry[symbol] = (prevCost + qty*price) / newQty
	crypto.streams.Add(symbol)
	crypto.syncHeldSnapshot()
	crypto.publishPositions()
}

func (crypto *Crypto) openShort(symbol string, qty, price float64) {
	if qty <= 0 {
		return
	}

	prevQty := crypto.shortInventory[symbol]
	prevCost := prevQty * crypto.avgEntry[symbol]
	newQty := prevQty + qty

	crypto.shortInventory[symbol] = newQty
	crypto.avgEntry[symbol] = (prevCost + qty*price) / newQty
	crypto.streams.Add(symbol)
	crypto.syncHeldSnapshot()
	crypto.publishPositions()
}

func (crypto *Crypto) closeShort(symbol string) {
	delete(crypto.shortInventory, symbol)
	delete(crypto.avgEntry, symbol)
	delete(crypto.armed, symbol+":"+string(trading.Buy))
	delete(crypto.armed, symbol+":"+string(trading.Sell))
	crypto.clearEntryConviction(symbol)
	crypto.streams.Remove(symbol)
	crypto.syncHeldSnapshot()
	crypto.publishPositions()
}

func (crypto *Crypto) closePosition(symbol string) {
	delete(crypto.inventory, symbol)
	delete(crypto.avgEntry, symbol)
	delete(crypto.armed, symbol+":"+string(trading.Buy))
	delete(crypto.armed, symbol+":"+string(trading.Sell))
	crypto.clearEntryConviction(symbol)
	crypto.streams.Remove(symbol)
	crypto.syncHeldSnapshot()
	crypto.publishPositions()
}

/*
observeBalances reconciles the trader's positions against an exchange balance SNAPSHOT
(the holdings frame the account stream emits on connect/reconnect in both paper and
live). It is how the trader recovers positions it did not open this session — opened
earlier, or held across a reconnect — instead of believing it is flat and re-deploying.
*/
func (crypto *Crypto) observeBalances(envelope map[string]any) {
	if envelope["channel"] != "holdings" {
		return
	}

	rows, ok := envelope["holdings"].([]map[string]any)

	if !ok {
		return
	}

	held := make(map[string]float64, len(rows))

	for _, row := range rows {
		symbol, _ := row["symbol"].(string)
		qty, _ := row["qty"].(float64)

		if symbol == "" || qty == 0 {
			continue
		}

		held[symbol] = qty
	}

	crypto.reconcilePositions(held)
}

/*
reconcilePositions makes the tracked positions agree with the exchange's held
holdings: it adopts any holding it is not already tracking and closes any tracked
position the exchange no longer shows. Positions it already tracks are left untouched,
so a reconnect snapshot never overwrites a known entry price with the current mark.
*/
func (crypto *Crypto) reconcilePositions(held map[string]float64) {
	for symbol, qty := range held {
		if qty > 0 {
			if _, tracked := crypto.inventory[symbol]; !tracked {
				crypto.adoptPosition(symbol, qty)
			}
		} else if qty < 0 {
			if _, tracked := crypto.shortInventory[symbol]; !tracked {
				crypto.adoptShort(symbol, math.Abs(qty))
			}
		}
	}

	stale := make([]string, 0)
	staleShorts := make([]string, 0)

	for symbol := range crypto.inventory {
		if qty, stillHeld := held[symbol]; !stillHeld || qty <= 0 {
			stale = append(stale, symbol)
		}
	}

	for symbol := range crypto.shortInventory {
		if qty, stillHeld := held[symbol]; !stillHeld || qty >= 0 {
			staleShorts = append(staleShorts, symbol)
		}
	}

	for _, symbol := range stale {
		crypto.closePosition(symbol)
	}

	for _, symbol := range staleShorts {
		crypto.closeShort(symbol)
	}
}

/*
adoptPosition takes ownership of a holding found on the exchange that the trader was
not tracking. The true entry price is unknowable from a balance snapshot, so the cost
basis is adopted at the current mark (backfilled by publishMarks if no quote is ready
yet); the position then reads ~breakeven and is managed from here by the universal
position manager.
*/
func (crypto *Crypto) adoptPosition(symbol string, qty float64) {
	errnie.Debug(fmt.Sprintf("Adopting untracked position: %s, qty: %f", symbol, qty))
	crypto.inventory[symbol] = qty
	crypto.avgEntry[symbol] = crypto.markFor(symbol)
	crypto.streams.Add(symbol)
	crypto.syncHeldSnapshot()
	crypto.publishPositions()
}

func (crypto *Crypto) adoptShort(symbol string, qty float64) {
	errnie.Debug(fmt.Sprintf("Adopting untracked short: %s, qty: %f", symbol, qty))
	crypto.shortInventory[symbol] = qty
	crypto.avgEntry[symbol] = crypto.markFor(symbol)
	crypto.streams.Add(symbol)
	crypto.syncHeldSnapshot()
	crypto.publishPositions()
}

// markFor returns the current mark price for a symbol from the quote cache, or 0 when
// no quote is available yet.
func (crypto *Crypto) markFor(symbol string) float64 {
	if crypto.quotes == nil {
		return 0
	}

	quote, ok := crypto.quotes.Snapshot(symbol)

	if !ok {
		return 0
	}

	return markPrice(quote)
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

	crypto.publishEquity()
}

/*
publishMarks streams the live mark price of each held symbol to the dashboard so the
open-positions panel can show real-time unrealized P&L. Positions are published only
when they open or close, so without a continuous mark the panel marks every position
at its own entry and reads a flat €0.00. Runs on the Tick goroutine, so the inventory
read is race-free.
*/
func (crypto *Crypto) publishMarks() {
	if crypto.quotes == nil || len(crypto.inventory) == 0 {
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

		// An adopted holding's cost basis is, by definition, the first market price we
		// can observe for it — its true entry is unrecoverable from a balance
		// snapshot. Set it here on that first mark (filled positions always carry a
		// real fill price, so this only ever initialises an adopted one). It also
		// anchors the protective triggers, so it is not gated on the UI.
		if crypto.avgEntry[symbol] <= 0 {
			crypto.avgEntry[symbol] = price
			crypto.publishPositions()
		}

		if crypto.ui == nil {
			continue
		}

		crypto.ui.Send(&qpool.QValue[any]{Value: map[string]any{
			"event":  "mark",
			"symbol": symbol,
			"price":  price,
		}})
	}
}

/*
publishEquity ships the realistic all-cash balance after market-selling every held
lot through the L2 book (SlippageFill) and paying the configured taker fee on each
leg. The dashboard shows this beside the wallet cash balance as "if I exit now".
*/
func (crypto *Crypto) publishEquity() {
	if crypto.ui == nil {
		return
	}

	cash := crypto.availableCash()
	capitalBase := crypto.capitalBaseValue()
	exitBalance, err := broker.ProjectExitBalance(
		cash,
		crypto.inventory,
		crypto.quotes,
		broker.TakerFeePctFromViper(),
	)

	if err != nil {
		return
	}

	crypto.ui.Send(&qpool.QValue[any]{Value: map[string]any{
		"event":        "equity",
		"cash":         cash,
		"exit_balance": exitBalance,
		"capital_base": capitalBase,
	}})
}

// markPrice values an open long at the bid — the price it could actually be sold into
// right now, and exactly where settle_position fills. Entries are buys that fill at
// the ask, so marking at the bid makes a fresh position open down the round-trip
// spread (realistic) instead of showing a phantom profit from a higher last/mid. When
// there is no bid the position cannot be honestly marked, so this returns a
// non-positive value and callers skip it — never a substitute price.
func markPrice(quote broker.Quote) float64 {
	return quote.Bid
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
