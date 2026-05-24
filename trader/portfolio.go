package trader

import (
	"context"
	"sync"
	"time"

	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/ui"
)

const (
	positionLong  = 1
	positionShort = -1
)

/*
ExecutionDecision is one scored entry candidate for the paper portfolio.
*/
type ExecutionDecision struct {
	Symbol         string
	Source         string
	Regime         string
	Reason         string
	Score          float64
	ExpectedReturn float64
	Runway         time.Duration
	Price          float64
	Side           int
}

/*
Position tracks one open paper trade.
*/
type Position struct {
	Symbol      string
	Source      string
	Regime      string
	Reason      string
	Score       float64
	Side        int
	EntryPrice  float64
	FillPrice   float64
	StopPrice   float64
	PeakPrice   float64
	NotionalEUR float64
	EntryFeeEUR float64
	TrailPct    float64
	BaseQty     float64
	OrderID     string
	StopOrderID string
	OpenedAt    time.Time
}

/*
PortfolioEvent is one lifecycle websocket payload.
*/
type PortfolioEvent struct {
	Name    string
	Payload map[string]any
}

/*
Portfolio owns open positions and paper wallet debits for the trader loop.
*/
type Portfolio struct {
	mu             sync.Mutex
	wallet         *Wallet
	broker         ExecutionBroker
	positions      map[string]*Position
	pendingEntries map[string]struct{}
	pendingExits   map[string]struct{}
	closedPnL      float64
	dailyClosedPnL float64
	dailyPnLDay    time.Time
	tradeCount     int
	wins           int
	ui             *qpool.BroadcastGroup
	riskReader     RiskReader
	exitAdvisor    ExitAdvisor
	trailRisk      *trailRiskFilter
	orderJournal   *OrderJournal
	haltReason     string
	store          *PortfolioStore
}

/*
NewPortfolio creates an empty paper portfolio bound to one wallet.
*/
func NewPortfolio(wallet *Wallet) *Portfolio {
	return &Portfolio{
		wallet:         wallet,
		broker:         NewPaperBroker(),
		positions:      make(map[string]*Position),
		pendingEntries: make(map[string]struct{}),
		pendingExits:   make(map[string]struct{}),
		trailRisk:      newTrailRiskFilter(),
	}
}

/*
BindBroker replaces the default paper broker with a live Kraken broker.
*/
func (portfolio *Portfolio) BindBroker(broker ExecutionBroker) {
	portfolio.mu.Lock()
	defer portfolio.mu.Unlock()

	if broker == nil {
		portfolio.broker = NewPaperBroker()

		return
	}

	portfolio.broker = broker
	portfolio.reconcileStopOrdersLocked()
}

func (portfolio *Portfolio) reconcileStopOrdersLocked() {
	if portfolio.orderJournal == nil {
		return
	}

	for symbol, position := range portfolio.positions {
		if position.StopOrderID != "" {
			continue
		}

		stopOrderID := portfolio.orderJournal.LatestStopOrderID(symbol)

		if stopOrderID == "" {
			continue
		}

		position.StopOrderID = stopOrderID
	}
}

/*
BindRiskReader wires live topology metrics for dynamic trailing stops.
*/
func (portfolio *Portfolio) BindRiskReader(reader RiskReader) {
	portfolio.mu.Lock()
	defer portfolio.mu.Unlock()

	portfolio.riskReader = reader
}

/*
StatusSnapshot is the portfolio slice of dashboard status telemetry.
*/
type StatusSnapshot struct {
	EquityEUR    float64
	CashEUR      float64
	ReservedEUR  float64
	ClosedPnLEUR float64
	TradeCount   int
	WinRate      float64
	OpenCount    int
	Positions    []map[string]any
}

/*
TradingAllowed reports whether new entries are permitted.
*/
func (portfolio *Portfolio) TradingAllowed() bool {
	portfolio.mu.Lock()
	defer portfolio.mu.Unlock()

	return portfolio.haltReason == ""
}

/*
HaltReason returns the active trading halt reason, if any.
*/
func (portfolio *Portfolio) HaltReason() string {
	portfolio.mu.Lock()
	defer portfolio.mu.Unlock()

	return portfolio.haltReason
}

func (portfolio *Portfolio) haltLocked(reason string) {
	if reason == "" {
		return
	}

	portfolio.haltReason = reason
}

/*
BindPortfolioStore wires portfolio persistence for paper restarts.
*/
func (portfolio *Portfolio) BindPortfolioStore(store *PortfolioStore) {
	portfolio.mu.Lock()
	defer portfolio.mu.Unlock()

	portfolio.store = store
}

func (portfolio *Portfolio) persistLocked() {
	if portfolio.store == nil {
		return
	}

	_ = portfolio.store.Save(portfolio)
}

/*
BindOrderJournal wires live order persistence for reconciliation.
*/
func (portfolio *Portfolio) BindOrderJournal(journal *OrderJournal) {
	portfolio.mu.Lock()
	defer portfolio.mu.Unlock()

	portfolio.orderJournal = journal
}

/*
BindExitAdvisor wires urgency scoring for early position closes.
*/
func (portfolio *Portfolio) BindExitAdvisor(advisor ExitAdvisor) {
	portfolio.mu.Lock()
	defer portfolio.mu.Unlock()

	portfolio.exitAdvisor = advisor
}

/*
BindUI wires lifecycle event publishing to the shared dashboard group.
*/
func (portfolio *Portfolio) BindUI(uiGroup *qpool.BroadcastGroup) {
	portfolio.mu.Lock()
	defer portfolio.mu.Unlock()

	portfolio.ui = uiGroup
}

/*
TryEnter opens one paper position when slot and wallet constraints allow.
*/
func (portfolio *Portfolio) TryEnter(
	ctx context.Context,
	now time.Time,
	decision ExecutionDecision,
	quotes QuoteReader,
) (*PortfolioEvent, bool) {
	plan, ok := portfolio.prepareEntry(now, decision, quotes)

	if !ok {
		return nil, false
	}

	brokerFill, err := portfolio.broker.Enter(ctx, plan.brokerRequest)

	portfolio.mu.Lock()
	defer portfolio.mu.Unlock()

	delete(portfolio.pendingEntries, decision.Symbol)

	if err != nil {
		portfolio.releaseEntryReservationLocked(plan.reservedEUR)

		return nil, false
	}

	if plan.brokerRequest.StopPrice > 0 &&
		plan.brokerRequest.Side == positionLong &&
		brokerFill.StopOrderID == "" {
		portfolio.releaseEntryReservationLocked(plan.reservedEUR)

		if portfolio.broker.Live() {
			portfolio.haltLocked("unconfirmed_stop")
		}

		return nil, false
	}

	event := portfolio.commitEntryLocked(now, decision, plan, brokerFill)

	if portfolio.broker.Live() && brokerFill.StopOrderID == "" {
		portfolio.haltLocked("unprotected_position")
	}

	return event, true
}

type entryPlan struct {
	brokerRequest BrokerEnterRequest
	trailPct      float64
	notional      float64
	reservedEUR   float64
}

func (portfolio *Portfolio) prepareEntry(
	now time.Time,
	decision ExecutionDecision,
	quotes QuoteReader,
) (entryPlan, bool) {
	side := decision.Side

	if side == 0 {
		side = positionLong
	}

	if side != positionLong && side != positionShort {
		return entryPlan{}, false
	}

	if decision.Symbol == "" || decision.Price <= 0 || decision.Score <= 0 {
		return entryPlan{}, false
	}

	portfolio.mu.Lock()
	defer portfolio.mu.Unlock()

	if side == positionShort && !portfolio.brokerSupportsShortLocked() {
		return entryPlan{}, false
	}

	if portfolio.haltReason != "" {
		return entryPlan{}, false
	}

	portfolio.rollDailyPnLLocked(now)

	if config.System.MaxDailyLossEUR > 0 &&
		portfolio.dailyClosedPnL <= -config.System.MaxDailyLossEUR {
		return entryPlan{}, false
	}

	if _, pending := portfolio.pendingEntries[decision.Symbol]; pending {
		return entryPlan{}, false
	}

	if len(portfolio.positions) >= config.System.MaxSlots {
		return entryPlan{}, false
	}

	if _, open := portfolio.positions[decision.Symbol]; open {
		return entryPlan{}, false
	}

	notional := portfolio.slotNotional()

	if notional < config.System.MinCostEUR {
		return entryPlan{}, false
	}

	last, bid, ask, _, ok := quotes.Quote(decision.Symbol)

	if !ok || last <= 0 {
		last = decision.Price
	}

	if config.System.MaxSpreadBPS > 0 {
		spread := quoteSpreadBPS(last, bid, ask)

		if spread <= 0 || spread > config.System.MaxSpreadBPS {
			return entryPlan{}, false
		}
	}

	bidLevels, askLevels := bookDepthFor(quotes, decision.Symbol)
	trailPct := clampTrailPct(trailPctFromQuoteRisk(
		last, bid, ask, decision.Symbol, portfolio.riskReader, portfolio.trailRisk,
	))
	estimatedStop := initialStop(last, trailPct, side)

	if lossAtStop(notional, trailPct) > config.System.MaxLossPerTradeEUR &&
		config.System.MaxLossPerTradeEUR > 0 {
		return entryPlan{}, false
	}

	estimatedFee := config.System.TakerFee(notional, portfolio.wallet.FeePct)
	reservedEUR := 0.0

	if side == positionLong {
		cost := notional + estimatedFee

		if portfolio.wallet == nil || portfolio.wallet.AvailableEUR() < cost {
			return entryPlan{}, false
		}

		if err := portfolio.wallet.ReserveEntry(cost); err != nil {
			return entryPlan{}, false
		}

		reservedEUR = cost
	}

	if side == positionShort {
		if portfolio.wallet == nil || portfolio.wallet.AvailableEUR() < notional {
			return entryPlan{}, false
		}
	}

	portfolio.pendingEntries[decision.Symbol] = struct{}{}

	return entryPlan{
		brokerRequest: BrokerEnterRequest{
			Symbol:      decision.Symbol,
			Side:        side,
			NotionalEUR: notional,
			Last:        last,
			Bid:         bid,
			Ask:         ask,
			StopPrice:   estimatedStop,
			FeePct:      portfolio.wallet.FeePct,
			BidLevels:   bidLevels,
			AskLevels:   askLevels,
		},
		trailPct:    trailPct,
		notional:    notional,
		reservedEUR: reservedEUR,
	}, true
}

func (portfolio *Portfolio) releaseEntryReservationLocked(reservedEUR float64) {
	if portfolio.wallet == nil {
		return
	}

	portfolio.wallet.ReleaseEntryReservation(reservedEUR)
}

func (portfolio *Portfolio) commitEntryLocked(
	now time.Time,
	decision ExecutionDecision,
	plan entryPlan,
	brokerFill BrokerFill,
) *PortfolioEvent {
	side := plan.brokerRequest.Side
	fill := brokerFill.FillPrice
	fee := brokerFill.FeeEUR
	stop := initialStop(fill, plan.trailPct, side)
	entryProceeds := spotProceedsEUR(brokerFill.BaseQty, fill)

	if entryProceeds <= 0 {
		entryProceeds = plan.notional
	}

	if side == positionLong {
		actualCost := entryProceeds + fee

		if err := portfolio.wallet.SettleEntryReservation(plan.reservedEUR, actualCost); err != nil {
			portfolio.haltLocked("entry_settlement_failed")
		}

		if err := portfolio.wallet.CreditBase(decision.Symbol, brokerFill.BaseQty); err != nil {
			portfolio.haltLocked("inventory_credit_failed")
		}
	}

	if side == positionShort {
		portfolio.wallet.Balance += entryProceeds - fee
	}

	position := &Position{
		Symbol:      decision.Symbol,
		Source:      decision.Source,
		Regime:      decision.Regime,
		Reason:      decision.Reason,
		Score:       decision.Score,
		Side:        side,
		EntryPrice:  plan.brokerRequest.Last,
		FillPrice:   fill,
		StopPrice:   stop,
		PeakPrice:   fill,
		NotionalEUR: entryProceeds,
		EntryFeeEUR: fee,
		TrailPct:    plan.trailPct,
		BaseQty:     brokerFill.BaseQty,
		OrderID:     brokerFill.OrderID,
		StopOrderID: brokerFill.StopOrderID,
		OpenedAt:    now,
	}

	portfolio.positions[decision.Symbol] = position

	if portfolio.orderJournal != nil {
		sideLabel := "long"

		if side == positionShort {
			sideLabel = "short"
		}

		portfolio.orderJournal.RecordEntry(OrderJournalEntry{
			Event:       "trade_enter",
			Symbol:      decision.Symbol,
			Side:        sideLabel,
			OrderID:     brokerFill.OrderID,
			StopOrderID: brokerFill.StopOrderID,
			NotionalEUR: plan.notional,
			FillPrice:   fill,
		})
	}

	portfolio.persistLocked()

	return portfolio.enterEvent(now, position)
}

func (portfolio *Portfolio) brokerSupportsShortLocked() bool {
	if portfolio.broker == nil {
		return false
	}

	return portfolio.broker.SupportsShort()
}

/*
Mark updates peaks, ratchets stops, and closes positions that hit exit rules.
*/
func (portfolio *Portfolio) Mark(
	ctx context.Context,
	now time.Time,
	quotes QuoteReader,
) []PortfolioEvent {
	type ratchetAction struct {
		position  *Position
		oldStop   float64
		last      float64
		amendStop bool
	}

	portfolio.mu.Lock()

	ratchetActions := make([]ratchetAction, 0, len(portfolio.positions))
	exitPlans := make([]exitPlan, 0, len(portfolio.positions))
	events := make([]PortfolioEvent, 0, len(portfolio.positions))

	for symbol, position := range portfolio.positions {
		if _, pending := portfolio.pendingExits[symbol]; pending {
			continue
		}

		last, bid, ask, _, ok := quotes.Quote(symbol)

		if !ok || last <= 0 {
			continue
		}

		if stopExit, ok := portfolio.pollLiveStopFillLocked(symbol, position); ok {
			feePct := 0.0

			if portfolio.wallet != nil {
				feePct = portfolio.wallet.FeePct
			}

			portfolio.pendingExits[symbol] = struct{}{}
			exitPlans = append(exitPlans, exitPlan{
				symbol:        symbol,
				reason:        "stop",
				position:      position,
				last:          last,
				bid:           bid,
				ask:           ask,
				feePct:        feePct,
				usePolledFill: true,
				polledFill:    stopExit,
			})

			continue
		}

		portfolio.updatePeak(position, last)

		trailPct := trailPctFromQuoteRisk(
			last, bid, ask, symbol, portfolio.riskReader, portfolio.trailRisk,
		)

		if trailPct > 0 {
			position.TrailPct = trailPct
		}

		newStop := trailingStop(position)
		oldStop := position.StopPrice
		amendStop := false

		if portfolio.ratchetStop(position, newStop) {
			amendStop = portfolio.broker != nil && position.StopOrderID != ""

			ratchetActions = append(ratchetActions, ratchetAction{
				position:  position,
				oldStop:   oldStop,
				last:      last,
				amendStop: amendStop,
			})
		}

		if portfolio.stopTriggered(position, last) {
			if portfolio.broker != nil &&
				portfolio.broker.Live() &&
				position.StopOrderID != "" {
				continue
			}

			if portfolio.broker != nil &&
				portfolio.broker.Live() &&
				position.StopOrderID == "" {
				portfolio.haltLocked("unprotected_stop")
			}

			feePct := 0.0

			if portfolio.wallet != nil {
				feePct = portfolio.wallet.FeePct
			}

			portfolio.pendingExits[symbol] = struct{}{}
			exitPlans = append(exitPlans, exitPlan{
				symbol:   symbol,
				reason:   "stop",
				position: position,
				last:     last,
				bid:      bid,
				ask:      ask,
				feePct:   feePct,
				stopExit: true,
			})

			continue
		}

		if !portfolio.canExit(now, position) {
			continue
		}

		if !portfolio.shouldExitEarly(position) {
			continue
		}

		feePct := 0.0

		if portfolio.wallet != nil {
			feePct = portfolio.wallet.FeePct
		}

		portfolio.pendingExits[symbol] = struct{}{}
		exitPlans = append(exitPlans, exitPlan{
			symbol:   symbol,
			reason:   portfolio.earlyExitReason(position),
			position: position,
			last:     last,
			bid:      bid,
			ask:      ask,
			feePct:   feePct,
		})
	}

	portfolio.mu.Unlock()

	for _, action := range ratchetActions {
		events = append(events, portfolio.ratchetEvent(now, action.position, action.oldStop, action.last))

		if !action.amendStop {
			continue
		}

		amendErr := portfolio.broker.AmendStop(ctx, BrokerAmendStopRequest{
			OrderID:      action.position.StopOrderID,
			TriggerPrice: action.position.StopPrice,
		})

		if amendErr != nil && portfolio.broker.Live() {
			portfolio.mu.Lock()
			portfolio.haltLocked("stop_amend_failed")
			portfolio.mu.Unlock()
		}
	}

	for _, plan := range exitPlans {
		event := portfolio.executeExit(ctx, now, plan, quotes)

		if event.Name == "" {
			continue
		}

		events = append(events, event)
	}

	return events
}

/*
Status returns mark-to-market portfolio telemetry for one quote snapshot.
*/
func (portfolio *Portfolio) Status(quotes QuoteReader) StatusSnapshot {
	portfolio.mu.Lock()
	defer portfolio.mu.Unlock()

	snapshot := StatusSnapshot{
		CashEUR:      walletBalance(portfolio.wallet),
		ReservedEUR:  walletReserved(portfolio.wallet),
		ClosedPnLEUR: portfolio.closedPnL,
		TradeCount:   portfolio.tradeCount,
		OpenCount:    len(portfolio.positions),
		Positions:    make([]map[string]any, 0, len(portfolio.positions)),
	}

	if portfolio.tradeCount > 0 {
		snapshot.WinRate = float64(portfolio.wins) / float64(portfolio.tradeCount)
	}

	equity := snapshot.CashEUR

	for _, position := range portfolio.positions {
		last := position.EntryPrice

		if quotes != nil {
			quoteLast, _, _, _, ok := quotes.Quote(position.Symbol)

			if ok && quoteLast > 0 {
				last = quoteLast
			}
		}

		equity += portfolio.netMarkValue(position, last)

		sideLabel := "long"

		if position.Side == positionShort {
			sideLabel = "short"
		}

		snapshot.Positions = append(snapshot.Positions, map[string]any{
			"symbol":       position.Symbol,
			"regime":       position.Regime,
			"side":         sideLabel,
			"entry_price":  position.EntryPrice,
			"stop_price":   position.StopPrice,
			"peak_price":   position.PeakPrice,
			"last_price":   last,
			"trail_pct":    position.TrailPct,
			"notional_eur": position.NotionalEUR,
			"open_pnl_eur": portfolio.realizedPnL(position, last),
			"opened_at":    position.OpenedAt.UTC().Format(time.RFC3339Nano),
		})
	}

	snapshot.EquityEUR = equity

	return snapshot
}

func (portfolio *Portfolio) slotNotional() float64 {
	if portfolio.wallet == nil || portfolio.wallet.AvailableEUR() <= 0 {
		return 0
	}

	slotPct := config.System.MaxSlotPct

	if slotPct <= 0 {
		slotPct = 5
	}

	available := portfolio.wallet.AvailableEUR()
	notional := available * slotPct / 100

	if notional > available {
		return available
	}

	return notional
}

func (portfolio *Portfolio) pollLiveStopFillLocked(
	symbol string,
	position *Position,
) (BrokerFill, bool) {
	if portfolio.broker == nil || !portfolio.broker.Live() || position.StopOrderID == "" {
		return BrokerFill{}, false
	}

	poller, ok := portfolio.broker.(FillPoller)

	if !ok {
		return BrokerFill{}, false
	}

	fill, ok := poller.PollFill(position.StopOrderID)

	if !ok {
		return BrokerFill{}, false
	}

	_ = symbol

	return fill, true
}

func (portfolio *Portfolio) shouldExitEarly(position *Position) bool {
	if portfolio.exitAdvisor == nil || position == nil {
		return false
	}

	urgency, _ := portfolio.exitAdvisor.ExitUrgency(position.Symbol, position.Side)
	threshold := config.System.ExitUrgencyThreshold

	if threshold <= 0 {
		threshold = 0.65
	}

	return urgency >= threshold
}

func (portfolio *Portfolio) earlyExitReason(position *Position) string {
	if portfolio.exitAdvisor == nil || position == nil {
		return "exhaustion"
	}

	_, reason := portfolio.exitAdvisor.ExitUrgency(position.Symbol, position.Side)

	if reason == "" {
		return "exhaustion"
	}

	return reason
}

func (portfolio *Portfolio) canExit(now time.Time, position *Position) bool {
	if position == nil || position.OpenedAt.IsZero() {
		return false
	}

	minHold := minHoldForRegime(position.Regime)

	return !now.Before(position.OpenedAt.Add(minHold))
}

func minHoldForRegime(regime string) time.Duration {
	switch regime {
	case "pump", "momentum":
		if config.System.ScalpHoldBeforeExit > 0 {
			return config.System.ScalpHoldBeforeExit
		}
	case "flow":
		if config.System.FlowHoldBeforeExit > 0 {
			return config.System.FlowHoldBeforeExit
		}
	}

	minHold := config.System.MinHoldBeforeRotate

	if minHold <= 0 {
		minHold = time.Minute
	}

	return minHold
}

type exitPlan struct {
	symbol        string
	reason        string
	position      *Position
	last          float64
	bid           float64
	ask           float64
	feePct        float64
	stopExit      bool
	usePolledFill bool
	polledFill    BrokerFill
}

func (portfolio *Portfolio) executeExit(
	ctx context.Context,
	now time.Time,
	action exitPlan,
	quotes QuoteReader,
) PortfolioEvent {
	if action.usePolledFill {
		portfolio.mu.Lock()

		delete(portfolio.pendingExits, action.symbol)

		position, open := portfolio.positions[action.symbol]

		portfolio.mu.Unlock()

		if !open {
			return PortfolioEvent{}
		}

		return portfolio.commitExitLocked(now, position, action.reason, action.polledFill.FillPrice)
	}

	if !portfolio.hasInventoryForExit(action.position) {
		portfolio.mu.Lock()
		delete(portfolio.pendingExits, action.symbol)
		portfolio.haltLocked("inventory_shortfall")
		portfolio.mu.Unlock()

		return PortfolioEvent{}
	}

	bidLevels, askLevels := bookDepthFor(quotes, action.symbol)

	exitRequest := BrokerExitRequest{
		Symbol:      action.symbol,
		Side:        action.position.Side,
		NotionalEUR: action.position.NotionalEUR,
		BaseQty:     action.position.BaseQty,
		Last:        action.last,
		Bid:         action.bid,
		Ask:         action.ask,
		FeePct:      action.feePct,
		BidLevels:   bidLevels,
		AskLevels:   askLevels,
	}

	if action.stopExit {
		exitRequest.StopExit = true
		exitRequest.StopPrice = action.position.StopPrice
		exitRequest.LimitPrice = StopLimitBelow(action.position.StopPrice)
		exitRequest.StopOrderID = action.position.StopOrderID
	}

	brokerFill, err := portfolio.broker.Exit(ctx, exitRequest)

	portfolio.mu.Lock()
	defer portfolio.mu.Unlock()

	delete(portfolio.pendingExits, action.symbol)

	position, open := portfolio.positions[action.symbol]

	if !open {
		return PortfolioEvent{}
	}

	exitFill := action.last

	if err == nil {
		exitFill = brokerFill.FillPrice
	}

	if err != nil && portfolio.broker.Live() {
		return portfolio.exitEvent(now, position, action.reason+"_failed", 0, exitFill)
	}

	return portfolio.commitExitLocked(now, position, action.reason, exitFill)
}

func (portfolio *Portfolio) commitExitLocked(
	now time.Time,
	position *Position,
	reason string,
	exitFill float64,
) PortfolioEvent {
	pnl := portfolio.realizedPnL(position, exitFill)

	if portfolio.wallet != nil {
		if position.Side == positionLong {
			if err := portfolio.wallet.DebitBase(position.Symbol, position.BaseQty); err != nil {
				portfolio.haltLocked("inventory_debit_failed")

				return portfolio.exitEvent(now, position, reason+"_inventory_failed", 0, exitFill)
			}
		}

		portfolio.wallet.Balance += portfolio.exitCashFlow(position, exitFill)
	}

	portfolio.closedPnL += pnl
	portfolio.dailyClosedPnL += pnl
	portfolio.tradeCount++

	if pnl > 0 {
		portfolio.wins++
	}

	delete(portfolio.positions, position.Symbol)
	portfolio.trailRisk.forget(position.Symbol)

	if portfolio.orderJournal != nil {
		sideLabel := "long"

		if position.Side == positionShort {
			sideLabel = "short"
		}

		portfolio.orderJournal.RecordEntry(OrderJournalEntry{
			Event:     "trade_exit",
			Symbol:    position.Symbol,
			Side:      sideLabel,
			FillPrice: exitFill,
			Reason:    reason,
		})
	}

	portfolio.persistLocked()

	return portfolio.exitEvent(now, position, reason, pnl, exitFill)
}

func (portfolio *Portfolio) hasInventoryForExit(position *Position) bool {
	if position == nil || position.Side != positionLong {
		return true
	}

	portfolio.mu.Lock()
	defer portfolio.mu.Unlock()

	if portfolio.wallet == nil {
		return false
	}

	return portfolio.wallet.AvailableBase(position.Symbol) >= position.BaseQty
}

func (portfolio *Portfolio) rollDailyPnLLocked(now time.Time) {
	day := now.UTC().Truncate(24 * time.Hour)

	if portfolio.dailyPnLDay == day {
		return
	}

	portfolio.dailyPnLDay = day
	portfolio.dailyClosedPnL = 0
}

func quoteSpreadBPS(last, bid, ask float64) float64 {
	if last <= 0 || bid <= 0 || ask <= 0 || ask < bid {
		return 0
	}

	return (ask - bid) / last * 10000
}

func (portfolio *Portfolio) netMarkValue(position *Position, last float64) float64 {
	gross := portfolio.markValue(position, last)
	feePct := 0.0

	if portfolio.wallet != nil {
		feePct = portfolio.wallet.FeePct
	}

	if position.Side == positionShort {
		exitCost := positionExitProceeds(position, last)

		if exitCost <= 0 {
			return gross
		}

		return gross - spotTakerFeeEUR(exitCost, feePct)
	}

	exitProceeds := gross

	if exitProceeds <= 0 {
		return 0
	}

	return exitProceeds - spotTakerFeeEUR(exitProceeds, feePct)
}

func (portfolio *Portfolio) realizedPnL(position *Position, exitFill float64) float64 {
	if position.Side == positionShort {
		entryProceeds := position.NotionalEUR
		exitCost := positionExitProceeds(position, exitFill)
		exitFee := spotTakerFeeEUR(exitCost, portfolio.wallet.FeePct)

		return entryProceeds - position.EntryFeeEUR - exitCost - exitFee
	}

	proceeds := positionExitProceeds(position, exitFill)
	exitFee := spotTakerFeeEUR(proceeds, portfolio.wallet.FeePct)
	net := proceeds - exitFee

	return net - position.NotionalEUR - position.EntryFeeEUR
}

func (portfolio *Portfolio) exitCashFlow(position *Position, exitFill float64) float64 {
	if position.Side == positionShort {
		exitCost := positionExitProceeds(position, exitFill)
		exitFee := spotTakerFeeEUR(exitCost, portfolio.wallet.FeePct)

		return -(exitCost + exitFee)
	}

	proceeds := positionExitProceeds(position, exitFill)
	fee := spotTakerFeeEUR(proceeds, portfolio.wallet.FeePct)

	return proceeds - fee
}

func (portfolio *Portfolio) markValue(position *Position, last float64) float64 {
	return positionMarkValue(position, last)
}

func initialStop(fill, trailPct float64, side int) float64 {
	if side == positionShort {
		return fill * (1 + trailPct/100)
	}

	return fill * (1 - trailPct/100)
}

func trailingStop(position *Position) float64 {
	if position.Side == positionShort {
		return position.PeakPrice * (1 + position.TrailPct/100)
	}

	return position.PeakPrice * (1 - position.TrailPct/100)
}

func (portfolio *Portfolio) updatePeak(position *Position, last float64) {
	if position.Side == positionShort {
		if last < position.PeakPrice {
			position.PeakPrice = last
		}

		return
	}

	if last > position.PeakPrice {
		position.PeakPrice = last
	}
}

func (portfolio *Portfolio) ratchetStop(position *Position, newStop float64) bool {
	if position.Side == positionShort {
		if newStop >= position.StopPrice {
			return false
		}

		position.StopPrice = newStop

		return true
	}

	if newStop <= position.StopPrice {
		return false
	}

	position.StopPrice = newStop

	return true
}

func (portfolio *Portfolio) stopTriggered(position *Position, last float64) bool {
	if position.Side == positionShort {
		return last >= position.StopPrice
	}

	return last <= position.StopPrice
}

func bookDepthFor(
	quotes QuoteReader,
	symbol string,
) (bids, asks []market.BookLevel) {
	fillReader, ok := quotes.(FillReader)

	if !ok || fillReader == nil {
		return nil, nil
	}

	bids, asks, _ = fillReader.BookDepth(symbol)

	return bids, asks
}

func (portfolio *Portfolio) enterEvent(
	now time.Time,
	position *Position,
) *PortfolioEvent {
	sideLabel := "long"

	if position.Side == positionShort {
		sideLabel = "short"
	}

	return &PortfolioEvent{
		Name: "trade_enter",
		Payload: map[string]any{
			"event":        "trade_enter",
			"ts":           now.UTC().Format(time.RFC3339Nano),
			"symbol":       position.Symbol,
			"regime":       position.Regime,
			"side":         sideLabel,
			"reason":       position.Reason,
			"score":        position.Score,
			"trail_pct":    position.TrailPct,
			"fill":         position.FillPrice,
			"stop":         position.StopPrice,
			"notional_eur": position.NotionalEUR,
			"last":         position.EntryPrice,
		},
	}
}

func (portfolio *Portfolio) ratchetEvent(
	now time.Time,
	position *Position,
	oldStop, last float64,
) PortfolioEvent {
	return PortfolioEvent{
		Name: "stop_ratchet",
		Payload: map[string]any{
			"event":    "stop_ratchet",
			"ts":       now.UTC().Format(time.RFC3339Nano),
			"symbol":   position.Symbol,
			"old_stop": oldStop,
			"new_stop": position.StopPrice,
			"peak":     position.PeakPrice,
			"last":     last,
		},
	}
}

func (portfolio *Portfolio) exitEvent(
	now time.Time,
	position *Position,
	reason string,
	pnl, exitFill float64,
) PortfolioEvent {
	hold := now.Sub(position.OpenedAt)
	sideLabel := "long"

	if position.Side == positionShort {
		sideLabel = "short"
	}

	return PortfolioEvent{
		Name: "trade_exit",
		Payload: map[string]any{
			"event":       "trade_exit",
			"ts":          now.UTC().Format(time.RFC3339Nano),
			"symbol":      position.Symbol,
			"regime":      position.Regime,
			"side":        sideLabel,
			"reason":      reason,
			"pnl_eur":     pnl,
			"hold_ms":     hold.Milliseconds(),
			"entry_price": position.FillPrice,
			"stop_price":  position.StopPrice,
			"peak_price":  position.PeakPrice,
			"exit_price":  exitFill,
		},
	}
}

/*
Emit publishes one lifecycle event outside portfolio locks.
*/
func (portfolio *Portfolio) Emit(event *PortfolioEvent) {
	if event == nil || portfolio.ui == nil {
		return
	}

	ui.SendEvent(portfolio.ui, event.Payload)
}

func clampTrailPct(trailPct float64) float64 {
	if trailPct <= 0 {
		trailPct = config.System.DefaultTrailPct
	}

	minTrail := config.System.MinTrailPct

	if minTrail <= 0 {
		minTrail = 0.15
	}

	maxTrail := config.System.MaxTrailPct

	if maxTrail <= 0 {
		maxTrail = 3
	}

	if trailPct < minTrail {
		return minTrail
	}

	if trailPct > maxTrail {
		return maxTrail
	}

	return trailPct
}

func lossAtStop(notional, trailPct float64) float64 {
	if notional <= 0 || trailPct <= 0 {
		return 0
	}

	return notional * trailPct / 100
}

func trailPctFromQuote(last, bid, ask float64) float64 {
	if last <= 0 || bid <= 0 || ask <= 0 || ask < bid {
		return 0
	}

	spreadPct := (ask - bid) / last * 100
	multiple := config.System.TrailSpreadMultiple

	if multiple <= 0 {
		multiple = 2
	}

	return spreadPct * multiple
}
