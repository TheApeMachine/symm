package trader

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/focus"
	"github.com/theapemachine/symm/kraken/order"
	decision "github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/wallet"
)

const (
	// riskFractionPerTrade is the share of free balance committed to one entry.
	riskFractionPerTrade = 0.02
	// stopFraction places the protective stop this far below entry.
	stopFraction = 0.03
	// takeFraction takes profit this far above entry.
	takeFraction = 0.05
	// maxConcurrentPositions caps simultaneous exposure across the book.
	maxConcurrentPositions = 5
	// predictionTargetReturn is the favorable move a perspective entry is a bet
	// on; it is the predicted return reported to the dashboard's prediction
	// chart and settled against the realized move on exit.
	predictionTargetReturn = 0.05
	// predictionHorizon is the runway shown for an open prediction.
	predictionHorizon = 30 * time.Minute
	// walletResendInterval republishes the wallet snapshot on a cadence so a
	// dashboard that connects after startup still sees the balance promptly (the
	// boot-time frame is fanned out to zero clients and lost).
	walletResendInterval = 2 * time.Second
)

// position is the trade desk's protective bracket for one open symbol: the price
// it bought at and the stop/target that close it. Exits are price-based and live
// here in the desk — risk is not a side-door.
type position struct {
	entry  float64
	stop   float64
	target float64
	mark   float64 // last seen price, re-marked to keep P/L fresh between ticks
}

/*
Crypto is the whole trade desk: it receives signal measurements, finds the
perspective they form for each symbol, and makes the buy/sell call. Risk is not
a side-door — it is the position sizing, the protective stop and take-profit
bracket, and the exposure cap, all computed at the moment the order is placed.
*/
type Crypto struct {
	ctx          context.Context
	cancel       context.CancelFunc
	pool         *qpool.Q
	measurements *qpool.Subscriber
	ui           *qpool.BroadcastGroup
	wallet       *wallet.Wallet
	tracker      *focus.Set
	mu           sync.RWMutex
	readings     map[string]map[perspectives.CategoryType]perspectives.Measurement
	positions    map[string]position
	open         atomic.Int64
	auditSeq     atomic.Uint64
}

func NewCrypto(
	ctx context.Context,
	pool *qpool.Q,
	tradingWallet *wallet.Wallet,
	tracker *focus.Set,
) *Crypto {
	ctx, cancel := context.WithCancel(ctx)

	crypto := &Crypto{
		ctx:       ctx,
		cancel:    cancel,
		pool:      pool,
		wallet:    tradingWallet,
		tracker:   tracker,
		readings:  make(map[string]map[perspectives.CategoryType]perspectives.Measurement),
		positions: make(map[string]position),
	}

	group := pool.CreateBroadcastGroup("measurements", 10*time.Millisecond)
	crypto.measurements = group.Subscribe("trader:measurements", 128)
	crypto.ui = pool.CreateBroadcastGroup("ui", 10*time.Millisecond)

	return crypto
}

// Tick consumes the measurement bus directly: every reading is recorded and the
// affected symbol's perspective is re-decided. No raw market feed — the price
// the trader needs to size and fill rides in on the measurement itself.
func (crypto *Crypto) Tick() error {
	wallet := time.NewTicker(walletResendInterval)
	defer wallet.Stop()

	for {
		select {
		case <-crypto.ctx.Done():
			return crypto.ctx.Err()
		case <-wallet.C:
			crypto.publishWallet()
			crypto.republishMarks()
		case value, ok := <-crypto.measurements.Incoming:
			if !ok || value.Value == nil {
				continue
			}

			measurement, measurementOK := value.Value.(perspectives.Measurement)

			if !measurementOK || measurement.Symbol == "" {
				continue
			}

			crypto.record(measurement)

			// A held symbol is marked-to-market in real time and managed by its
			// protective bracket; an open slot is offered to the entry playbooks.
			if _, held := crypto.wallet.PositionBindingFor(baseOf(measurement.Symbol)); held {
				crypto.remark(measurement.Symbol, measurement.Last)
				crypto.manageExit(measurement.Symbol, measurement.Last)
				continue
			}

			crypto.decide(measurement.Symbol, measurement.Last)
		}
	}
}

func (crypto *Crypto) record(measurement perspectives.Measurement) {
	crypto.mu.Lock()
	defer crypto.mu.Unlock()

	set := crypto.readings[measurement.Symbol]

	if set == nil {
		set = make(map[perspectives.CategoryType]perspectives.Measurement)
		crypto.readings[measurement.Symbol] = set
	}

	set[measurement.Category] = measurement
}

// decide forms the symbol's perspective from its readings and acts on the verdict.
func (crypto *Crypto) decide(symbol string, last float64) {
	crypto.mu.RLock()
	set := crypto.readings[symbol]
	measurements := make([]perspectives.Measurement, 0, len(set))

	for _, measurement := range set {
		measurements = append(measurements, measurement)
	}

	crypto.mu.RUnlock()

	action, _ := decision.Decide(measurements)

	if action != nil && *action == perspectives.ActionEnter {
		crypto.enter(symbol, last, entryReason(measurements))
	}
}

// entryReason narrates which signal triggered an entry: the strongest drive-class
// reading among the measurements, with its source and SNR.
func entryReason(measurements []perspectives.Measurement) string {
	best, found := strongestDrive(measurements)

	if !found {
		return "entry authorized"
	}

	return fmt.Sprintf(
		"%s.%s snr=%.2f", best.Source.String(), best.Category.String(), best.SNR,
	)
}

// strongestDrive returns the highest-SNR drive-class reading (the categories the
// drive playbook enters on).
func strongestDrive(measurements []perspectives.Measurement) (perspectives.Measurement, bool) {
	var best perspectives.Measurement
	found := false

	for _, measurement := range measurements {
		drive := measurement.Category == perspectives.CategoryAggressiveDrive ||
			measurement.Category == perspectives.CategoryHiddenAbsorption

		if drive && measurement.SNR > best.SNR {
			best = measurement
			found = true
		}
	}

	return best, found
}

// enter opens a position: it sizes by free balance, caps concurrent exposure, and
// records the protective stop/target bracket that the desk will close against.
func (crypto *Crypto) enter(symbol string, last float64, reason string) {
	if crypto.open.Load() >= maxConcurrentPositions || last <= 0 {
		return
	}

	notional := crypto.wallet.BalanceCopy() * riskFractionPerTrade

	if notional <= 0 {
		return
	}

	buy := broker.Buy{
		Symbol:    symbol,
		Notional:  notional,
		Quote:     broker.Quote{Last: last, Bid: last, Ask: last, At: time.Now()},
		StopPrice: last * (1 - stopFraction),
	}

	fill, err := buy.FillPaper(crypto.wallet)

	if err != nil {
		errnie.Error(err)
		return
	}

	crypto.wallet.BindPosition(baseOf(symbol), wallet.PositionBinding{
		Source:      "perspective",
		PredictedAt: time.Now(),
	})
	crypto.open.Add(1)
	crypto.tracker.Add(symbol)

	crypto.mu.Lock()
	crypto.positions[symbol] = position{
		entry:  fill.Price,
		stop:   fill.Price * (1 - stopFraction),
		target: fill.Price * (1 + takeFraction),
		mark:   fill.Price,
	}
	crypto.mu.Unlock()

	crypto.publishAudit("entry", symbol, fmt.Sprintf("bought %.2f %s — %s", notional, crypto.wallet.Snapshot().Currency, reason))
	crypto.publishFill(fill)
	crypto.publishWallet()
	crypto.publishPrediction(symbol)
}

// manageExit closes a held position when price reaches its stop or target. This
// is the risk bracket: the perspective layer never has to signal an exit.
func (crypto *Crypto) manageExit(symbol string, last float64) {
	crypto.mu.RLock()
	bracket, tracked := crypto.positions[symbol]
	crypto.mu.RUnlock()

	if !tracked || last <= 0 {
		return
	}

	if last > bracket.stop && last < bracket.target {
		return
	}

	trigger := "stop"

	if last >= bracket.target {
		trigger = "target"
	}

	base := baseOf(symbol)
	binding, _ := crypto.wallet.PositionBindingFor(base)
	entry := crypto.wallet.AvgEntryFor(base)

	sell := broker.Sell{
		Symbol: symbol,
		Quote:  broker.Quote{Last: last, Bid: last, Ask: last, At: time.Now()},
	}

	fill, err := sell.FillPaper(crypto.wallet)

	if err != nil {
		errnie.Error(err)
		return
	}

	crypto.wallet.ClearPosition(base)
	crypto.open.Add(-1)
	crypto.tracker.Remove(symbol)

	crypto.mu.Lock()
	delete(crypto.positions, symbol)
	crypto.mu.Unlock()

	realized := realizedReturn(entry, fill.Price)
	crypto.publishAudit("exit", symbol, fmt.Sprintf("%s hit at %.6g — return %.2f%%", trigger, last, realized*100))
	crypto.publishFill(fill)
	crypto.publishWallet()
	crypto.publishPredictionSettled(symbol, realized, binding.PredictedAt)
}

// remark records the latest price for a held symbol and marks it to market, so
// P/L tracks live whenever fresh measurements arrive.
func (crypto *Crypto) remark(symbol string, last float64) {
	if last <= 0 {
		return
	}

	crypto.mu.Lock()
	if bracket, ok := crypto.positions[symbol]; ok {
		bracket.mark = last
		crypto.positions[symbol] = bracket
	}
	crypto.mu.Unlock()

	crypto.publishMark(symbol, last)
}

// republishMarks re-marks every open position at its last known price on a
// cadence, so a dashboard that connects late — or a thin coin that has stopped
// printing — still shows live P/L rather than a bare inventory row.
func (crypto *Crypto) republishMarks() {
	crypto.mu.RLock()
	marks := make(map[string]float64, len(crypto.positions))

	for symbol, bracket := range crypto.positions {
		marks[symbol] = bracket.mark
	}

	crypto.mu.RUnlock()

	for symbol, mark := range marks {
		crypto.publishMark(symbol, mark)
	}
}

// publishMark marks a held position to market so the dashboard shows live P/L,
// independent of whether the symbol produces OHLC candles.
func (crypto *Crypto) publishMark(symbol string, last float64) {
	if last <= 0 {
		return
	}

	crypto.ui.Send(&qpool.QValue[any]{Value: map[string]any{
		"event":  "mark",
		"ts":     time.Now().UTC().Format(time.RFC3339Nano),
		"symbol": symbol,
		"price":  last,
	}})
}

// publishAudit narrates a trade-desk decision to the dashboard's audit log.
func (crypto *Crypto) publishAudit(auditEvent, symbol, reason string) {
	crypto.ui.Send(&qpool.QValue[any]{Value: map[string]any{
		"event":       "audit",
		"ts":          time.Now().UTC().Format(time.RFC3339Nano),
		"seq":         crypto.auditSeq.Add(1),
		"audit_event": auditEvent,
		"symbol":      symbol,
		"source":      "trader",
		"reason":      reason,
	}})
}

// publishPrediction reports a new entry thesis to the dashboard prediction chart.
func (crypto *Crypto) publishPrediction(symbol string) {
	now := time.Now()

	crypto.ui.Send(&qpool.QValue[any]{Value: map[string]any{
		"event":     "prediction",
		"ts":        now.UTC().Format(time.RFC3339Nano),
		"symbol":    symbol,
		"source":    "perspective",
		"value":     predictionTargetReturn,
		"due_at":    now.Add(predictionHorizon).UTC().Format(time.RFC3339Nano),
		"runway_ms": predictionHorizon.Milliseconds(),
	}})
}

// publishPredictionSettled reports the realized outcome of an entry thesis.
func (crypto *Crypto) publishPredictionSettled(
	symbol string, actualReturn float64, predictedAt time.Time,
) {
	now := time.Now()

	crypto.ui.Send(&qpool.QValue[any]{Value: map[string]any{
		"event":            "prediction_settled",
		"ts":               now.UTC().Format(time.RFC3339Nano),
		"symbol":           symbol,
		"source":           "perspective",
		"predicted_at":     predictedAt.UTC().Format(time.RFC3339Nano),
		"due_at":           now.UTC().Format(time.RFC3339Nano),
		"predicted_return": predictionTargetReturn,
		"actual_return":    actualReturn,
		"error":            actualReturn - predictionTargetReturn,
	}})
}

// realizedReturn is the fractional move from entry to exit price.
func realizedReturn(entry, exit float64) float64 {
	if entry <= 0 {
		return 0
	}

	return (exit - entry) / entry
}

// publishFill ships one execution to the dashboard's trades panel.
func (crypto *Crypto) publishFill(fill order.Fill) {
	crypto.ui.Send(&qpool.QValue[any]{Value: fill})
}

// publishWallet ships the current balance and inventory snapshot to the wallet
// and trades panels.
func (crypto *Crypto) publishWallet() {
	snapshot := crypto.wallet.Snapshot()

	crypto.ui.Send(&qpool.QValue[any]{Value: map[string]any{
		"event":       "wallet",
		"ts":          time.Now().UTC().Format(time.RFC3339Nano),
		"Currency":    snapshot.Currency,
		"Balance":     snapshot.Balance,
		"ReservedEUR": snapshot.ReservedEUR,
		"FeePct":      snapshot.FeePct,
		"Inventory":   snapshot.Inventory,
		"AvgEntry":    snapshot.AvgEntry,
		"Marks":       snapshot.Marks,
	}})
}

// ResendWallet publishes the current wallet snapshot. The booter calls it at
// startup so the dashboard shows the opening balance before any trade happens.
func (crypto *Crypto) ResendWallet() {
	crypto.publishWallet()
}

func (crypto *Crypto) Close() error {
	crypto.cancel()
	return nil
}

// baseOf returns the base currency of a "BASE/QUOTE" pair symbol.
func baseOf(symbol string) string {
	if base, _, found := strings.Cut(symbol, "/"); found {
		return base
	}

	return symbol
}
