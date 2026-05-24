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
	ctx            context.Context
	cancel         context.CancelFunc
	pool           *qpool.Q
	wallet         *Wallet
	portfolio      *Portfolio
	prices         QuoteReader
	signals        []engine.Signal
	pairStates     sync.Map
	telemetry      *Telemetry
	tickSeq        atomic.Uint64
	feedbackSink   func(engine.PredictionFeedback)
	candidates     CandidateStore
	decisionEngine DecisionEngine
	lastDecision   DecisionSnapshot
	rescoreCount   int
}

/*
NewCrypto creates a new crypto trader.
*/
func NewCrypto(
	ctx context.Context,
	pool *qpool.Q,
	wallet *Wallet,
	prices QuoteReader,
	signals ...engine.Signal,
) (*Crypto, error) {
	ctx, cancel := context.WithCancel(ctx)

	crypto := &Crypto{
		ctx:            ctx,
		cancel:         cancel,
		pool:           pool,
		wallet:         wallet,
		portfolio:      NewPortfolio(wallet),
		prices:         prices,
		signals:        signals,
		pairStates:     sync.Map{},
		candidates:     NewCandidateStore(),
		decisionEngine: DecisionEngine{},
	}

	crypto.portfolio.BindRiskReader(NewSignalRiskBoard(signals...))

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
BindFeedbackSink records settled prediction feedback for offline evaluation.
*/
func (crypto *Crypto) BindFeedbackSink(
	sink func(engine.PredictionFeedback),
) {
	crypto.feedbackSink = sink
}

/*
Rescore runs one scan, drain, forecast settlement, and execution tick at now.
*/
func (crypto *Crypto) Rescore(now time.Time) error {
	crypto.beginRescoreTick()

	if err := crypto.scanSignals(now); err != nil {
		return err
	}

	crypto.processTick(now)
	crypto.settleDuePredictions(now)
	crypto.runExecution(now)

	return nil
}

/*
DecisionSnapshot returns the latest decision snapshot for telemetry.
*/
func (crypto *Crypto) DecisionSnapshot() DecisionSnapshot {
	return crypto.lastDecision
}

func (crypto *Crypto) beginRescoreTick() {
	crypto.candidates.Reset()

	if crypto.telemetry != nil {
		crypto.telemetry.BeginTick()
	}
}

/*
PendingPredictionCount returns unresolved forecasts across all pair states.
*/
func (crypto *Crypto) PendingPredictionCount() int {
	total := 0

	crypto.pairStates.Range(func(_, value any) bool {
		state, ok := value.(*PairState)

		if !ok || state == nil {
			return true
		}

		total += state.PendingCount()

		return true
	})

	return total
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
			crypto.beginRescoreTick()

			if err := crypto.scanSignals(now); err != nil {
				return err
			}

			crypto.processTick(now)
			crypto.settleDuePredictions(now)
			crypto.runExecution(now)

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
		crypto.noteCandidate(measurement)

		if crypto.telemetry != nil {
			crypto.telemetry.NoteMeasurement(measurement)
		}
	}

	for _, feedback := range result.feedback {
		crypto.applyFeedback(feedback)
	}
}

func (crypto *Crypto) noteCandidate(measurement engine.Measurement) {
	for _, pair := range measurement.Pairs {
		symbol := asset.Symbol(pair)

		if symbol == "" {
			continue
		}

		crypto.candidates.Note(SignalCandidate{
			Symbol:         symbol,
			Source:         measurement.Source,
			Regime:         measurement.Regime,
			Reason:         measurement.Reason,
			Confidence:     measurement.Confidence,
			ExpectedReturn: measurement.ExpectedReturn,
			Runway:         measurement.Runway,
			Direction:      measurement.Type.Direction(),
		})
	}
}

func (crypto *Crypto) settleDuePredictions(now time.Time) {
	if crypto.prices == nil {
		return
	}

	crypto.pairStates.Range(func(_, value any) bool {
		state, ok := value.(*PairState)

		if !ok || state == nil {
			return true
		}

		symbol := state.Symbol()

		if symbol == "" {
			return true
		}

		quote, ok := crypto.quotePrice(symbol)

		if !ok || quote <= 0 {
			return true
		}

		for _, feedback := range state.SettleDue(now, quote) {
			crypto.applyFeedback(feedback)
		}

		return true
	})
}

func (crypto *Crypto) mergeLiveCandidates() {
	for _, signal := range crypto.signals {
		reader, ok := signal.(engine.LiveScoreReader)

		if !ok {
			continue
		}

		peak := reader.PeakReading()

		if peak.Score <= 0 || peak.Symbol == "" {
			continue
		}

		crypto.candidates.Note(SignalCandidate{
			Symbol:     peak.Symbol,
			Source:     signal.Source(),
			Regime:     "live",
			Reason:     "track",
			Confidence: peak.Score,
			Direction:  1,
		})
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
	if crypto.feedbackSink != nil {
		crypto.feedbackSink(feedback)
	}

	for _, signal := range crypto.signals {
		receiver, ok := signal.(engine.FeedbackReceiver)

		if !ok {
			continue
		}

		receiver.ApplyFeedback(feedback)
	}
}

func (crypto *Crypto) runExecution(now time.Time) {
	if crypto.portfolio == nil || crypto.prices == nil {
		return
	}

	for _, event := range crypto.portfolio.Mark(now, crypto.prices) {
		markEvent := event
		crypto.portfolio.Emit(&markEvent)
	}

	crypto.mergeLiveCandidates()

	warming := crypto.rescoreCount < config.System.MinWarmPulses
	crypto.rescoreCount++
	crypto.lastDecision = crypto.decisionEngine.Build(
		crypto.candidates, crypto.prices, warming,
	)

	if warming {
		return
	}

	for _, evaluation := range crypto.lastDecision.Evaluations {
		if !evaluation.Allow {
			continue
		}

		last, ok := crypto.quotePrice(evaluation.Symbol)

		if !ok {
			continue
		}

		decision := ExecutionDecision{
			Symbol:         evaluation.Symbol,
			Regime:         evaluation.Regime,
			Reason:         evaluation.Reason,
			Score:          evaluation.CombinedScore,
			ExpectedReturn: evaluation.ExpectedReturn,
			Runway:         evaluation.Runway,
			Price:          last,
		}

		if evaluation.Side == "short" {
			decision.Side = positionShort
		}

		if event, ok := crypto.portfolio.TryEnter(now, decision, crypto.prices); ok {
			crypto.portfolio.Emit(event)
		}
	}
}
