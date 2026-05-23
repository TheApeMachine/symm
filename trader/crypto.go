package trader

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
)

/*
Crypto paper-trades Kraken microstructure signals with trailing stops.
Live order placement is not wired; all fills go through PaperWallet.
*/
type Crypto struct {
	ctx             context.Context
	cancel          context.CancelFunc
	pool            *qpool.Q
	wallet          *Wallet
	prices          PriceReader
	publisher       Publisher
	engineStats     EngineStats
	signals         []engine.Signal
	holds           map[string]position
	records         map[string]symbolRecord
	priorCandidates map[string]struct{}
	closedPnL       float64
	tradeCount      int
	winCount        int
	pulseSeq        atomic.Int64
	statusSink      func() map[string]any
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
		ctx:             ctx,
		cancel:          cancel,
		pool:            pool,
		wallet:          wallet,
		prices:          prices,
		publisher:       publisher,
		signals:         signals,
		holds:           make(map[string]position),
		records:         make(map[string]symbolRecord),
		priorCandidates: make(map[string]struct{}),
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

	exitTicker := time.NewTicker(config.System.ExitEvery)
	defer exitTicker.Stop()

	rescoreTicker := time.NewTicker(config.System.RescoreEvery)
	defer rescoreTicker.Stop()

	for {
		select {
		case <-crypto.ctx.Done():
			return crypto.ctx.Err()
		case <-exitTicker.C:
			if err := crypto.markExits(); err != nil {
				return err
			}
		case <-rescoreTicker.C:
			batch := crypto.drainMeasurements()
			crypto.decide(batch)
		}
	}
}

func (crypto *Crypto) drainMeasurements() []engine.Measurement {
	if crypto.pool == nil || len(crypto.signals) <= 1 {
		return crypto.drainMeasurementsSequential()
	}

	type drainResult struct {
		items []engine.Measurement
	}

	results := make([][]engine.Measurement, len(crypto.signals))
	waitGroup := sync.WaitGroup{}
	waitGroup.Add(len(crypto.signals))

	for index, signal := range crypto.signals {
		signalIndex := index
		activeSignal := signal

		resultCh := crypto.pool.Schedule(
			fmt.Sprintf("measure-%d-%d", signalIndex, crypto.pulseSeq.Load()),
			func(ctx context.Context) (any, error) {
				items := make([]engine.Measurement, 0)

				for measurement := range activeSignal.Measure(ctx) {
					items = append(items, measurement)
				}

				return items, nil
			},
		)

		go func() {
			defer waitGroup.Done()

			result := <-resultCh

			if result == nil {
				return
			}

			if result.Error != nil {
				errnie.Error(result.Error)

				return
			}

			items, ok := result.Value.([]engine.Measurement)

			if !ok {
				errnie.Error(fmt.Errorf("invalid measurement batch type: %T", result.Value))

				return
			}

			results[signalIndex] = items
		}()
	}

	waitGroup.Wait()

	batch := make([]engine.Measurement, 0)

	for _, items := range results {
		batch = append(batch, items...)
	}

	return batch
}

func (crypto *Crypto) drainMeasurementsSequential() []engine.Measurement {
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
	line := batchEntryLine(candidates)
	peakConfidence := batchPeakConfidence(candidates)

	crypto.publishEnginePulse(batch, candidates)
	crypto.publishScoreboard(batch, candidates, line)
	crypto.publishDecisionTrace(batch, candidates, line)

	if len(candidates) == 0 {
		crypto.priorCandidates = nil
		return
	}

	nextCandidates := make(map[string]struct{}, len(candidates))

	for _, candidate := range candidates {
		nextCandidates[candidate.symbol] = struct{}{}
	}

	if !crypto.readyForTrading() {
		crypto.priorCandidates = nextCandidates
		return
	}

	if !crypto.tradingSolvent() {
		crypto.priorCandidates = nextCandidates
		return
	}

	for _, candidate := range candidates {
		if _, seen := crypto.priorCandidates[candidate.symbol]; !seen {
			continue
		}

		if !crypto.meetsEntryLine(candidate, line) {
			continue
		}

		crypto.tryEnter(candidate, peakConfidence)
	}

	crypto.priorCandidates = nextCandidates
	crypto.publishStatus()
}

/*
Close closes the crypto trader and cancels open positions at market.
*/
func (crypto *Crypto) Close() error {
	crypto.closeAllPositions("shutdown")
	crypto.cancel()

	return nil
}

func (crypto *Crypto) closeAllPositions(reason string) {
	for symbol, hold := range crypto.holds {
		exitFill, ok := crypto.exitFill(symbol)

		if !ok || exitFill <= 0 {
			delete(crypto.holds, symbol)
			continue
		}

		crypto.closePosition(symbol, hold, exitFill, reason)
	}
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

	if !crypto.canAffordEntry(notional) {
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
			"event":        "trade_enter",
			"ts":           time.Now().UTC().Format(time.RFC3339Nano),
			"symbol":       candidate.symbol,
			"regime":       candidate.regime,
			"reason":       candidate.reason,
			"score":        candidate.confidence,
			"trail_pct":    trail,
			"fill":         entryFill,
			"stop":         stop,
			"notional_eur": notional,
			"last":         entryFill,
		})
	}
}
