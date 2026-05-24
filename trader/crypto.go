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
	ctx        context.Context
	cancel     context.CancelFunc
	pool       *qpool.Q
	wallet     *Wallet
	prices     PriceReader
	signals    []engine.Signal
	pairStates sync.Map
	telemetry  *Telemetry
	tickSeq    atomic.Uint64
}

/*
NewCrypto creates a new crypto trader.
*/
func NewCrypto(
	ctx context.Context,
	pool *qpool.Q,
	wallet *Wallet,
	prices PriceReader,
	signals ...engine.Signal,
) (*Crypto, error) {
	ctx, cancel := context.WithCancel(ctx)

	crypto := &Crypto{
		ctx:        ctx,
		cancel:     cancel,
		pool:       pool,
		wallet:     wallet,
		prices:     prices,
		signals:    signals,
		pairStates: sync.Map{},
	}

	return crypto, errnie.Require(map[string]any{
		"ctx":        ctx,
		"cancel":     cancel,
		"wallet":     wallet,
		"prices":     prices,
		"signals":    signals,
		"pairStates": &crypto.pairStates,
	})
}

func (crypto *Crypto) sourceScores() map[string]float64 {
	scores := make(map[string]float64, len(crypto.signals))

	for _, signal := range crypto.signals {
		reader, ok := signal.(engine.LiveScoreReader)

		if !ok {
			continue
		}

		scores[signal.Source()] = reader.LiveScore()
	}

	return scores
}

/*
Run runs the crypto trader on a single scheduler: scan → drain → decide.
*/
func (crypto *Crypto) Run() error {
	if crypto.telemetry != nil {
		crypto.telemetry.Publish(crypto.wallet, crypto)
	}

	rescoreTicker := time.NewTicker(config.System.RescoreEvery)
	defer rescoreTicker.Stop()

	for {
		select {
		case <-crypto.ctx.Done():
			return crypto.ctx.Err()
		case now := <-rescoreTicker.C:
			if crypto.telemetry != nil {
				crypto.telemetry.BeginTick()
			}

			if err := crypto.scanSignals(now); err != nil {
				return err
			}

			crypto.processTick(now)

			if crypto.telemetry != nil {
				crypto.telemetry.Publish(crypto.wallet, crypto)
			}
		}
	}
}

type signalTickResult struct {
	measurements []engine.Measurement
	feedback     []engine.PredictionFeedback
}

/*
updatePairStates ingests one signal measurement per pair.
*/
func (crypto *Crypto) updatePairStates(
	measurement engine.Measurement, now time.Time,
) []engine.PredictionFeedback {
	pending := make([]engine.PredictionFeedback, 0)

	for _, pair := range measurement.Pairs {
		state := crypto.pairState(pair)

		if state == nil {
			continue
		}

		state.Update(measurement)

		if measurement.ExpectedReturn <= 0 {
			continue
		}

		state.RecordPrediction(now, measurement)

		symbol := asset.Symbol(pair)
		quotePrice, ok := crypto.quotePrice(symbol)

		if !ok {
			continue
		}

		state.AnchorPending(quotePrice)
		pending = append(pending, state.SettleDue(now, quotePrice)...)
	}

	return pending
}

func (crypto *Crypto) scanSignals(now time.Time) error {
	if crypto.pool == nil {
		return crypto.scanSignalsSequential(now)
	}

	resultChannels := make([]chan *qpool.QValue[any], len(crypto.signals))

	for index, signal := range crypto.signals {
		jobIndex := index
		jobSignal := signal
		jobID := fmt.Sprintf("crypto:scan:%d:%d", jobIndex, crypto.nextJobID())
		resultChannels[jobIndex] = crypto.pool.Schedule(jobID, func(context.Context) (any, error) {
			return nil, jobSignal.Scan(now)
		})
	}

	for _, resultChannel := range resultChannels {
		result := <-resultChannel

		if result == nil {
			continue
		}

		if result.Error != nil {
			return result.Error
		}
	}

	return nil
}

func (crypto *Crypto) scanSignalsSequential(now time.Time) error {
	for _, signal := range crypto.signals {
		if err := signal.Scan(now); err != nil {
			return err
		}
	}

	return nil
}

func (crypto *Crypto) processTick(now time.Time) {
	if crypto.pool == nil {
		crypto.processTickSequential(now)

		return
	}

	resultChannels := make([]chan *qpool.QValue[any], len(crypto.signals))

	for index, signal := range crypto.signals {
		jobIndex := index
		jobSignal := signal
		jobID := fmt.Sprintf("crypto:tick:%d:%d", jobIndex, crypto.nextJobID())
		resultChannels[jobIndex] = crypto.pool.Schedule(jobID, func(context.Context) (any, error) {
			return crypto.drainSignal(jobSignal, now), nil
		})
	}

	for _, resultChannel := range resultChannels {
		result := <-resultChannel

		if result == nil || result.Error != nil {
			continue
		}

		tickResult, ok := result.Value.(signalTickResult)

		if !ok {
			continue
		}

		crypto.applyTickResult(tickResult)
	}
}

func (crypto *Crypto) processTickSequential(now time.Time) {
	for _, signal := range crypto.signals {
		crypto.applyTickResult(crypto.drainSignal(signal, now))
	}
}

func (crypto *Crypto) drainSignal(
	signal engine.Signal, now time.Time,
) signalTickResult {
	result := signalTickResult{}

	for measurement := range signal.Measure(crypto.ctx) {
		result.measurements = append(result.measurements, measurement)
		result.feedback = append(
			result.feedback,
			crypto.updatePairStates(measurement, now)...,
		)
	}

	return result
}

func (crypto *Crypto) applyTickResult(result signalTickResult) {
	for _, measurement := range result.measurements {
		if crypto.telemetry != nil {
			crypto.telemetry.NoteMeasurement(measurement)
		}
	}

	for _, feedback := range result.feedback {
		crypto.applyFeedback(feedback)
	}
}

func (crypto *Crypto) nextJobID() uint64 {
	return crypto.tickSeq.Add(1)
}

func (crypto *Crypto) pairState(pair asset.Pair) *PairState {
	symbol := asset.Symbol(pair)

	if symbol == "" {
		return nil
	}

	if loaded, ok := crypto.pairStates.Load(symbol); ok {
		return loaded.(*PairState)
	}

	state := NewPairState(pair)
	loaded, _ := crypto.pairStates.LoadOrStore(symbol, state)

	return loaded.(*PairState)
}

func (crypto *Crypto) quotePrice(symbol string) (float64, bool) {
	if crypto.prices == nil || symbol == "" {
		return 0, false
	}

	return crypto.prices.Last(symbol)
}

func (crypto *Crypto) applyFeedback(feedback engine.PredictionFeedback) {
	for _, signal := range crypto.signals {
		receiver, ok := signal.(engine.FeedbackReceiver)

		if !ok {
			continue
		}

		receiver.ApplyFeedback(feedback)
	}
}
