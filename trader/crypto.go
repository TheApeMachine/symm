package trader

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
)

/*
Crypto is the real crypto-currency trader, which uses actual
money from the user's account.
*/
type Crypto struct {
	ctx         context.Context
	cancel      context.CancelFunc
	pool        *qpool.Q
	wallet      *Wallet
	prices      PriceReader
	publisher   Publisher
	engineStats EngineStats
	signals     []engine.Signal
	holds       map[string]position
	records     map[string]symbolRecord
	closedPnL   float64
	tradeCount  int
	winCount    int
	pulseSeq    atomic.Int64
	statusSink  func() map[string]any
}

type position struct {
	pair       asset.Pair
	notional   float64
	entryPrice float64
	entryFee   float64
	enteredAt  time.Time
	confidence float64
	regime     string
	reason     string
	trailPct   float64
	stopPrice  float64
	peakPrice  float64
}

/*
NewCrypto creates a new crypto trader.
*/
func NewCrypto(
	ctx context.Context,
	pool *qpool.Q,
	wallet *Wallet,
	prices PriceReader,
	publisher Publisher,
	signals ...engine.Signal,
) (*Crypto, error) {
	ctx, cancel := context.WithCancel(ctx)

	if publisher == nil {
		publisher = NoopPublisher()
	}

	crypto := &Crypto{
		ctx:       ctx,
		cancel:    cancel,
		pool:      pool,
		wallet:    wallet,
		prices:    prices,
		publisher: publisher,
		signals:   signals,
		holds:     make(map[string]position),
		records:   make(map[string]symbolRecord),
	}

	crypto.statusSink = crypto.statusEvent

	return crypto, errnie.Require(map[string]any{
		"ctx":     ctx,
		"cancel":  cancel,
		"wallet":  wallet,
		"signals": signals,
	})
}

/*
SetEngineStats wires live counters for engine_pulse events.
*/
func (crypto *Crypto) SetEngineStats(stats EngineStats) {
	crypto.engineStats = stats
}

/*
StatusSnapshot returns the latest wallet and position telemetry.
*/
func (crypto *Crypto) StatusSnapshot() map[string]any {
	if crypto.statusSink == nil {
		return crypto.statusEvent()
	}

	return crypto.statusSink()
}

/*
Run runs the crypto trader.
*/
func (crypto *Crypto) Run() error {
	for _, signal := range crypto.signals {
		signal.Run()
	}

	crypto.publishStatus()

	ticker := time.NewTicker(config.System.RescoreEvery)
	defer ticker.Stop()

	for {
		select {
		case <-crypto.ctx.Done():
			return crypto.ctx.Err()
		case <-ticker.C:
			if err := crypto.markExits(); err != nil {
				return err
			}

			batch := crypto.drainMeasurements()
			crypto.decide(batch)
		}
	}
}

func (crypto *Crypto) drainMeasurements() []engine.Measurement {
	batch := make([]engine.Measurement, 0)

	for _, signal := range crypto.signals {
		for measurement := range signal.Measure(crypto.ctx) {
			batch = append(batch, measurement)
		}
	}

	return batch
}

func (crypto *Crypto) decide(batch []engine.Measurement) {
	for _, measurement := range batch {
		if measurement.Err != nil {
			errnie.Error(measurement.Err)
		}
	}

	candidates := crypto.rankCandidates(batch)
	peakConfidence := 0.0

	if len(candidates) > 0 {
		peakConfidence = candidates[0].confidence
	}

	crypto.publishEnginePulse(batch, candidates)
	crypto.publishScoreboard(batch, candidates, peakConfidence)
	crypto.publishDecisionTrace(batch, candidates, peakConfidence)

	if len(candidates) == 0 {
		return
	}

	for _, candidate := range candidates {
		crypto.tryEnter(candidate, peakConfidence)
	}

	crypto.publishStatus()
}

/*
Close closes the crypto trader.
*/
func (crypto *Crypto) Close() error {
	crypto.cancel()
	return nil
}

func (crypto *Crypto) tryEnter(candidate tradeCandidate, peakConfidence float64) {
	if _, held := crypto.holds[candidate.symbol]; held {
		return
	}

	if !crypto.canEnter(candidate) {
		return
	}

	notional := crypto.entryNotional(candidate.confidence, peakConfidence)

	if notional <= 0 {
		return
	}

	entryFill, trail, ok := crypto.entryFill(candidate.symbol)

	if !ok || entryFill <= 0 {
		return
	}

	entryFee := config.System.TakerFee(notional, crypto.wallet.FeePct)
	crypto.wallet.Balance -= notional + entryFee

	stop := stopFromEntry(entryFill, trail)

	crypto.holds[candidate.symbol] = position{
		pair:       candidate.pair,
		notional:   notional,
		entryPrice: entryFill,
		entryFee:   entryFee,
		enteredAt:  time.Now(),
		confidence: candidate.confidence,
		regime:     candidate.regime,
		reason:     candidate.reason,
		trailPct:   trail,
		stopPrice:  stop,
		peakPrice:  entryFill,
	}

	errnie.Info(fmt.Sprintf(
		"paper_enter symbol=%s regime=%s notional=%.4f confidence=%.4f support=%d fee=%.4f wins=%d",
		candidate.symbol, candidate.regime, notional, candidate.confidence, candidate.support, entryFee,
		crypto.records[candidate.symbol].wins,
	))

	if crypto.publisher != nil {
		crypto.publisher.Emit(map[string]any{
			"event":          "trade_enter",
			"ts":             time.Now().UTC().Format(time.RFC3339Nano),
			"symbol":         candidate.symbol,
			"regime":         candidate.regime,
			"reason":         candidate.reason,
			"score":          candidate.confidence,
			"trail_pct":      trail,
			"fill":           entryFill,
			"stop":           stop,
			"notional_eur":   notional,
			"last":           entryFill,
		})
	}
}

func (crypto *Crypto) entryNotional(confidence, peakConfidence float64) float64 {
	if crypto.wallet.Balance <= 0 || config.System.MaxSlotPct <= 0 {
		return 0
	}

	if confidence <= 0 || peakConfidence <= 0 {
		return 0
	}

	weight := confidence / peakConfidence
	notional := crypto.wallet.Balance * config.System.MaxSlotPct / 100 * weight

	if config.System.MinCostEUR > 0 && notional < config.System.MinCostEUR {
		return 0
	}

	return notional
}
