package trader

import (
	"sync"
	"time"

	"github.com/theapemachine/symm/config"
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
	EntryPrice  float64
	FillPrice   float64
	StopPrice   float64
	PeakPrice   float64
	NotionalEUR float64
	TrailPct    float64
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
PortfolioStream publishes trade lifecycle events to the dashboard.
*/
type PortfolioStream interface {
	TradeEnter(payload map[string]any)
	TradeExit(payload map[string]any)
	StopRatchet(payload map[string]any)
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
Portfolio owns open positions and paper wallet debits for the trader loop.
*/
type Portfolio struct {
	mu         sync.Mutex
	wallet     *Wallet
	positions  map[string]*Position
	closedPnL  float64
	tradeCount int
	wins       int
	stream     PortfolioStream
}

/*
NewPortfolio creates an empty paper portfolio bound to one wallet.
*/
func NewPortfolio(wallet *Wallet) *Portfolio {
	return &Portfolio{
		wallet:    wallet,
		positions: make(map[string]*Position),
	}
}

/*
BindStream wires lifecycle event publishing for trade telemetry.
*/
func (portfolio *Portfolio) BindStream(stream PortfolioStream) {
	portfolio.mu.Lock()
	defer portfolio.mu.Unlock()

	portfolio.stream = stream
}

/*
TryEnter opens one long paper position when slot and wallet constraints allow.
*/
func (portfolio *Portfolio) TryEnter(
	now time.Time,
	decision ExecutionDecision,
	quotes QuoteReader,
) (*PortfolioEvent, bool) {
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

	fill := config.System.SlippagePrice(last, bid, ask, "buy", config.System.SlippageBPS)

	if fill <= 0 {
		return nil, false
	}

	fee := config.System.TakerFee(notional, portfolio.wallet.FeePct)
	cost := notional + fee

	if portfolio.wallet == nil || portfolio.wallet.Balance < cost {
		return nil, false
	}

	trailPct := trailPctFromQuote(last, bid, ask)
	stop := fill * (1 - trailPct/100)

	portfolio.wallet.Balance -= cost

	position := &Position{
		Symbol:      decision.Symbol,
		Source:      decision.Source,
		Regime:      decision.Regime,
		Reason:      decision.Reason,
		Score:       decision.Score,
		EntryPrice:  last,
		FillPrice:   fill,
		StopPrice:   stop,
		PeakPrice:   fill,
		NotionalEUR: notional,
		TrailPct:    trailPct,
		OpenedAt:    now,
	}

	portfolio.positions[decision.Symbol] = position

	event := portfolio.enterEvent(now, position)
	portfolio.emitLocked(event)

	return event, true
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

		if last > position.PeakPrice {
			position.PeakPrice = last
		}

		trailPct := trailPctFromQuote(last, bid, ask)

		if trailPct > 0 {
			position.TrailPct = trailPct
		}

		newStop := position.PeakPrice * (1 - position.TrailPct/100)

		if newStop > position.StopPrice {
			oldStop := position.StopPrice
			position.StopPrice = newStop

			event := portfolio.ratchetEvent(now, position, oldStop, last)
			events = append(events, event)
			portfolio.emitLocked(&event)
		}

		if !portfolio.canExit(now, position) {
			continue
		}

		if last > position.StopPrice {
			continue
		}

		exitEvent := portfolio.closeLocked(now, symbol, position, last, bid, ask, "stop")
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
		last, _, _, _, ok := quotes.Quote(position.Symbol)

		if !ok || last <= 0 {
			last = position.EntryPrice
		}

		markValue := position.NotionalEUR * (last / position.FillPrice)
		equity += markValue

		snapshot.Positions = append(snapshot.Positions, map[string]any{
			"symbol":        position.Symbol,
			"regime":        position.Regime,
			"entry_price":   position.EntryPrice,
			"stop_price":    position.StopPrice,
			"peak_price":    position.PeakPrice,
			"last_price":    last,
			"trail_pct":     position.TrailPct,
			"notional_eur":  position.NotionalEUR,
			"opened_at":     position.OpenedAt.UTC().Format(time.RFC3339Nano),
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

func (portfolio *Portfolio) canExit(now time.Time, position *Position) bool {
	if position == nil || position.OpenedAt.IsZero() {
		return false
	}

	minHold := config.System.MinHoldBeforeRotate

	if minHold <= 0 {
		minHold = time.Minute
	}

	return !now.Before(position.OpenedAt.Add(minHold))
}

func (portfolio *Portfolio) closeLocked(
	now time.Time,
	symbol string,
	position *Position,
	last, bid, ask float64,
	reason string,
) PortfolioEvent {
	exitFill := config.System.SlippagePrice(last, bid, ask, "sell", config.System.SlippageBPS)
	proceeds := position.NotionalEUR * (exitFill / position.FillPrice)
	fee := config.System.TakerFee(proceeds, portfolio.wallet.FeePct)
	net := proceeds - fee
	pnl := net - position.NotionalEUR

	if portfolio.wallet != nil {
		portfolio.wallet.Balance += net
	}

	portfolio.closedPnL += pnl
	portfolio.tradeCount++

	if pnl > 0 {
		portfolio.wins++
	}

	delete(portfolio.positions, symbol)

	event := portfolio.exitEvent(now, position, reason, pnl, exitFill)
	portfolio.emitLocked(&event)

	return event
}

func (portfolio *Portfolio) enterEvent(
	now time.Time,
	position *Position,
) *PortfolioEvent {
	return &PortfolioEvent{
		Name: "trade_enter",
		Payload: map[string]any{
			"event":         "trade_enter",
			"ts":            now.UTC().Format(time.RFC3339Nano),
			"symbol":        position.Symbol,
			"regime":        position.Regime,
			"reason":        position.Reason,
			"score":         position.Score,
			"trail_pct":     position.TrailPct,
			"fill":          position.FillPrice,
			"stop":          position.StopPrice,
			"notional_eur":  position.NotionalEUR,
			"last":          position.EntryPrice,
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
			"event":     "stop_ratchet",
			"ts":        now.UTC().Format(time.RFC3339Nano),
			"symbol":    position.Symbol,
			"old_stop":  oldStop,
			"new_stop":  position.StopPrice,
			"peak":      position.PeakPrice,
			"last":      last,
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

	return PortfolioEvent{
		Name: "trade_exit",
		Payload: map[string]any{
			"event":        "trade_exit",
			"ts":           now.UTC().Format(time.RFC3339Nano),
			"symbol":       position.Symbol,
			"regime":       position.Regime,
			"reason":       reason,
			"pnl_eur":      pnl,
			"hold_ms":      hold.Milliseconds(),
			"entry_price":  position.FillPrice,
			"stop_price":   position.StopPrice,
			"peak_price":   position.PeakPrice,
			"exit_price":   exitFill,
		},
	}
}

func (portfolio *Portfolio) emitLocked(event *PortfolioEvent) {
	if event == nil || portfolio.stream == nil {
		return
	}

	switch event.Name {
	case "trade_enter":
		portfolio.stream.TradeEnter(event.Payload)
	case "trade_exit":
		portfolio.stream.TradeExit(event.Payload)
	case "stop_ratchet":
		portfolio.stream.StopRatchet(event.Payload)
	}
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
