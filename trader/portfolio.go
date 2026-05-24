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
	Symbol       string
	Source       string
	Regime       string
	Reason       string
	Score        float64
	Side         int
	EntryPrice   float64
	FillPrice    float64
	StopPrice    float64
	PeakPrice    float64
	NotionalEUR  float64
	EntryFeeEUR  float64
	TrailPct     float64
	BaseQty      float64
	OrderID      string
	StopOrderID  string
	OpenedAt     time.Time
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
	mu          sync.Mutex
	wallet      *Wallet
	broker      ExecutionBroker
	positions   map[string]*Position
	closedPnL   float64
	tradeCount  int
	wins        int
	ui          *qpool.BroadcastGroup
	riskReader  RiskReader
	exitAdvisor ExitAdvisor
	trailRisk   *trailRiskFilter
}

/*
NewPortfolio creates an empty paper portfolio bound to one wallet.
*/
func NewPortfolio(wallet *Wallet) *Portfolio {
	return &Portfolio{
		wallet:    wallet,
		broker:    NewPaperBroker(),
		positions: make(map[string]*Position),
		trailRisk: newTrailRiskFilter(),
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
	ClosedPnLEUR float64
	TradeCount   int
	WinRate      float64
	OpenCount    int
	Positions    []map[string]any
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
	now time.Time,
	decision ExecutionDecision,
	quotes QuoteReader,
) (*PortfolioEvent, bool) {
	side := decision.Side

	if side == 0 {
		side = positionLong
	}

	if side != positionLong && side != positionShort {
		return nil, false
	}

	if decision.Symbol == "" || decision.Price <= 0 || decision.Score <= 0 {
		return nil, false
	}

	portfolio.mu.Lock()
	defer portfolio.mu.Unlock()

	if len(portfolio.positions) >= config.System.MaxSlots {
		return nil, false
	}

	if _, open := portfolio.positions[decision.Symbol]; open {
		return nil, false
	}

	notional := portfolio.slotNotional()

	if notional < config.System.MinCostEUR {
		return nil, false
	}

	last, bid, ask, _, ok := quotes.Quote(decision.Symbol)

	if !ok || last <= 0 {
		last = decision.Price
	}

	bidLevels, askLevels := bookDepthFor(quotes, decision.Symbol)
	trailPct := clampTrailPct(trailPctFromQuoteRisk(
		last, bid, ask, decision.Symbol, portfolio.riskReader, portfolio.trailRisk,
	))
	estimatedStop := initialStop(last, trailPct, side)

	if lossAtStop(notional, trailPct) > config.System.MaxLossPerTradeEUR &&
		config.System.MaxLossPerTradeEUR > 0 {
		return nil, false
	}

	estimatedFee := config.System.TakerFee(notional, portfolio.wallet.FeePct)

	if side == positionLong {
		cost := notional + estimatedFee

		if portfolio.wallet == nil || portfolio.wallet.Balance < cost {
			return nil, false
		}
	}

	if side == positionShort {
		if portfolio.wallet == nil || portfolio.wallet.Balance < notional {
			return nil, false
		}
	}

	brokerFill, err := portfolio.broker.Enter(context.Background(), BrokerEnterRequest{
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
	})

	if err != nil {
		return nil, false
	}

	fill := brokerFill.FillPrice
	fee := brokerFill.FeeEUR
	stop := initialStop(fill, trailPct, side)

	if side == positionLong {
		portfolio.wallet.Balance -= notional + fee
	}

	if side == positionShort {
		portfolio.wallet.Balance += notional - fee
	}

	position := &Position{
		Symbol:      decision.Symbol,
		Source:      decision.Source,
		Regime:      decision.Regime,
		Reason:      decision.Reason,
		Score:       decision.Score,
		Side:        side,
		EntryPrice:  last,
		FillPrice:   fill,
		StopPrice:   stop,
		PeakPrice:   fill,
		NotionalEUR: notional,
		EntryFeeEUR: fee,
		TrailPct:    trailPct,
		BaseQty:     brokerFill.BaseQty,
		OrderID:     brokerFill.OrderID,
		StopOrderID: brokerFill.StopOrderID,
		OpenedAt:    now,
	}

	portfolio.positions[decision.Symbol] = position

	return portfolio.enterEvent(now, position), true
}

/*
Mark updates peaks, ratchets stops, and closes positions that hit exit rules.
*/
func (portfolio *Portfolio) Mark(now time.Time, quotes QuoteReader) []PortfolioEvent {
	portfolio.mu.Lock()
	defer portfolio.mu.Unlock()

	events := make([]PortfolioEvent, 0, len(portfolio.positions))

	for symbol, position := range portfolio.positions {
		last, bid, ask, _, ok := quotes.Quote(symbol)

		if !ok || last <= 0 {
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

		if portfolio.ratchetStop(position, newStop) {
			event := portfolio.ratchetEvent(now, position, oldStop, last)
			events = append(events, event)

			if portfolio.broker != nil && portfolio.broker.Live() && position.StopOrderID != "" {
				_ = portfolio.broker.AmendStop(context.Background(), BrokerAmendStopRequest{
					OrderID:      position.StopOrderID,
					TriggerPrice: position.StopPrice,
				})
			}
		}

		if !portfolio.canExit(now, position) {
			continue
		}

		if portfolio.shouldExitEarly(position) {
			exitEvent := portfolio.closeLocked(
				now, symbol, position, last, bid, ask, quotes, portfolio.earlyExitReason(position),
			)
			events = append(events, exitEvent)

			continue
		}

		if !portfolio.stopTriggered(position, last) {
			continue
		}

		exitEvent := portfolio.closeLocked(now, symbol, position, last, bid, ask, quotes, "stop")
		events = append(events, exitEvent)
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
	if portfolio.wallet == nil || portfolio.wallet.Balance <= 0 {
		return 0
	}

	slotPct := config.System.MaxSlotPct

	if slotPct <= 0 {
		slotPct = 5
	}

	notional := portfolio.wallet.Balance * slotPct / 100

	if notional > portfolio.wallet.Balance {
		return portfolio.wallet.Balance
	}

	return notional
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

func (portfolio *Portfolio) closeLocked(
	now time.Time,
	symbol string,
	position *Position,
	last, bid, ask float64,
	quotes QuoteReader,
	reason string,
) PortfolioEvent {
	fillSide := "sell"

	if position.Side == positionShort {
		fillSide = "buy"
	}

	bidLevels, askLevels := bookDepthFor(quotes, symbol)
	exitFill := config.System.SlippageFill(
		last, bid, ask, fillSide, config.System.SlippageBPS,
		position.NotionalEUR, bidLevels, askLevels,
	)

	pnl := portfolio.realizedPnL(position, exitFill)

	if portfolio.wallet != nil {
		portfolio.wallet.Balance += portfolio.exitCashFlow(position, exitFill)
	}

	portfolio.closedPnL += pnl
	portfolio.tradeCount++

	if pnl > 0 {
		portfolio.wins++
	}

	delete(portfolio.positions, symbol)
	portfolio.trailRisk.forget(symbol)

	event := portfolio.exitEvent(now, position, reason, pnl, exitFill)

	return event
}

func (portfolio *Portfolio) netMarkValue(position *Position, last float64) float64 {
	gross := portfolio.markValue(position, last)

	if position.Side == positionShort {
		exitCost := position.NotionalEUR * (last / position.FillPrice)

		if exitCost <= 0 {
			return gross
		}

		return gross - config.System.TakerFee(exitCost, portfolio.wallet.FeePct)
	}

	exitProceeds := gross

	if exitProceeds <= 0 {
		return 0
	}

	return exitProceeds - config.System.TakerFee(exitProceeds, portfolio.wallet.FeePct)
}

func (portfolio *Portfolio) realizedPnL(position *Position, exitFill float64) float64 {
	if position.Side == positionShort {
		entryProceeds := position.NotionalEUR
		exitCost := position.NotionalEUR * (exitFill / position.FillPrice)
		exitFee := config.System.TakerFee(exitCost, portfolio.wallet.FeePct)

		return entryProceeds - position.EntryFeeEUR - exitCost - exitFee
	}

	proceeds := position.NotionalEUR * (exitFill / position.FillPrice)
	exitFee := config.System.TakerFee(proceeds, portfolio.wallet.FeePct)
	net := proceeds - exitFee

	return net - position.NotionalEUR - position.EntryFeeEUR
}

func (portfolio *Portfolio) exitCashFlow(position *Position, exitFill float64) float64 {
	if position.Side == positionShort {
		exitCost := position.NotionalEUR * (exitFill / position.FillPrice)
		exitFee := config.System.TakerFee(exitCost, portfolio.wallet.FeePct)

		return -(exitCost + exitFee)
	}

	proceeds := position.NotionalEUR * (exitFill / position.FillPrice)
	fee := config.System.TakerFee(proceeds, portfolio.wallet.FeePct)

	return proceeds - fee
}

func (portfolio *Portfolio) markValue(position *Position, last float64) float64 {
	if position.Side == positionShort {
		unrealized := position.NotionalEUR * (position.FillPrice - last) / position.FillPrice

		return unrealized
	}

	return position.NotionalEUR * (last / position.FillPrice)
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
