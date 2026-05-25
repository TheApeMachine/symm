package trader

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
	"github.com/theapemachine/symm/ui"
)

/*
Scorer measures signals, settles forecasts, and publishes candidate frames.
*/
type Scorer struct {
	ctx              context.Context
	cancel           context.CancelFunc
	pool             *qpool.Q
	uiBroadcast      *qpool.BroadcastGroup
	candidates       *qpool.BroadcastGroup
	prices           QuoteReader
	market           engine.MarketReader
	signals          []engine.Signal
	pairStates       sync.Map
	feedbackSink     func(engine.PredictionFeedback)
	executable       CandidateStore
	liveCandidates   CandidateStore
	sourceTrust      *SourceTrustStore
	returnModel      *ReturnModel
	forecastReject   map[string]int
	rescoreCount     int
	rescoreTicker    *time.Ticker
	lastMeasurements []engine.Measurement
}

/*
NewScorer wires signal measurement and candidate publishing.
*/
func NewScorer(
	ctx context.Context,
	pool *qpool.Q,
	ui *qpool.BroadcastGroup,
	candidates *qpool.BroadcastGroup,
	prices QuoteReader,
	market engine.MarketReader,
	signals ...engine.Signal,
) (*Scorer, error) {
	ctx, cancel := context.WithCancel(ctx)

	scorer := &Scorer{
		ctx:            ctx,
		cancel:         cancel,
		pool:           pool,
		uiBroadcast:    ui,
		candidates:     candidates,
		prices:         prices,
		market:         market,
		signals:        signals,
		pairStates:     sync.Map{},
		executable:     NewCandidateStore(),
		liveCandidates: NewCandidateStore(),
		sourceTrust:    NewSourceTrustStore(),
		returnModel:    NewReturnModel(),
		forecastReject: make(map[string]int),
	}

	return scorer, errnie.Require(map[string]any{
		"ctx":        ctx,
		"cancel":     cancel,
		"pool":       pool,
		"candidates": candidates,
		"prices":     prices,
		"signals":    signals,
	})
}

/*
BindFeedbackSink records settled prediction feedback for offline evaluation.
*/
func (scorer *Scorer) BindFeedbackSink(
	sink func(engine.PredictionFeedback),
) {
	scorer.feedbackSink = sink
}

/*
CreditWarmPulses advances the warm-up counter after OHLC bootstrap.
*/
func (scorer *Scorer) CreditWarmPulses(credit int) {
	if credit <= 0 {
		return
	}

	scorer.rescoreCount += credit
}

/*
PendingPredictionCount returns unresolved forecasts across all pair states.
*/
func (scorer *Scorer) PendingPredictionCount() int {
	total := 0

	scorer.pairStates.Range(func(_, value any) bool {
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
ReturnModel exposes the scorer-owned return model for tests.
*/
func (scorer *Scorer) ReturnModel() *ReturnModel {
	return scorer.returnModel
}

/*
Start arms the rescore ticker.
*/
func (scorer *Scorer) Start() error {
	if scorer.rescoreTicker != nil {
		return nil
	}

	scorer.rescoreTicker = time.NewTicker(config.System.RescoreEvery)

	return nil
}

/*
State reports whether the scorer is ready for rescore ticks.
*/
func (scorer *Scorer) State() engine.State {
	return engine.READY
}

/*
Tick runs one scoring cycle when the interval elapses.
*/
func (scorer *Scorer) Tick() error {
	select {
	case <-scorer.ctx.Done():
		return scorer.ctx.Err()
	case <-scorer.rescoreTicker.C:
		scorer.runScoreTick()

		return nil
	default:
		return nil
	}
}

/*
Close stops the scorer context and rescore ticker.
*/
func (scorer *Scorer) Close() error {
	scorer.cancel()

	if scorer.rescoreTicker != nil {
		scorer.rescoreTicker.Stop()
	}

	return nil
}

/*
Rescore runs one full scoring tick for tests.
*/
func (scorer *Scorer) Rescore(now time.Time) error {
	scorer.runScoreTickAt(now)

	return nil
}

func (scorer *Scorer) runScoreTick() {
	scorer.runScoreTickAt(time.Now().UTC())
}

func (scorer *Scorer) runScoreTickAt(now time.Time) {
	scorer.executable.Reset()
	scorer.resetForecastRejects()

	tickResult := scorer.processSignals(now)

	scorer.settleDuePredictions(now)
	scorer.mergeLiveCandidates()
	scorer.publishEnginePulse(tickResult)
	scorer.publishCandidateFrame(tickResult)
}

type signalTickResult struct {
	measurements []engine.Measurement
	feedback     []engine.PredictionFeedback
}

func (scorer *Scorer) measureContext() context.Context {
	return scorer.ctx
}

func (scorer *Scorer) processSignals(now time.Time) signalTickResult {
	measureCtx := scorer.measureContext()
	result := signalTickResult{}

	for _, signal := range scorer.signals {
		engine.DrainTicks(measureCtx)
		tickResult := scorer.collectMeasurements(signal, now)
		scorer.applyTickResult(tickResult)
		result.measurements = append(result.measurements, tickResult.measurements...)
		result.feedback = append(result.feedback, tickResult.feedback...)
	}

	scorer.lastMeasurements = append([]engine.Measurement(nil), result.measurements...)

	return result
}

func (scorer *Scorer) collectMeasurements(
	signal engine.Signal, now time.Time,
) signalTickResult {
	result := signalTickResult{}

	for measurement := range signal.Measure(scorer.measureContext(), now) {
		result.measurements = append(result.measurements, measurement)
		result.feedback = append(
			result.feedback,
			scorer.updatePairStates(measurement, now)...,
		)
	}

	if reader, ok := signal.(engine.MeanConfidenceReader); ok && scorer.uiBroadcast != nil {
		ui.Publish(scorer.uiBroadcast, "signal_score", map[string]any{
			"source":     signal.Source(),
			"confidence": reader.MeanConfidence(),
		})
	}

	return result
}

func (scorer *Scorer) processSignal(signal engine.Signal, now time.Time) error {
	scorer.applyTickResult(scorer.collectMeasurements(signal, now))

	return nil
}

func (scorer *Scorer) updatePairStates(
	measurement engine.Measurement, now time.Time,
) []engine.PredictionFeedback {
	pending := make([]engine.PredictionFeedback, 0)

	for _, pair := range measurement.Pairs {
		state := scorer.pairState(pair)

		if state == nil {
			continue
		}

		symbol := asset.Symbol(pair)

		state.Update(measurement)

		forecast, reason := BuildSignalForecastReason(
			measurement, scorer.prices, symbol, scorer.returnModel,
		)

		if reason != "" {
			scorer.recordForecastReject(measurement.Source, reason)
			scorer.recordCalibrationProbe(state, measurement, now, symbol, reason)
			continue
		}

		state.ApplyForecast(forecast)

		baselineQuote, ok := scorer.quotePrice(symbol)

		if !ok {
			continue
		}

		state.RecordPrediction(now, measurement, forecast, baselineQuote)
		pending = append(pending, state.SettleDue(now, baselineQuote)...)
	}

	return pending
}

func (scorer *Scorer) applyTickResult(result signalTickResult) {
	for _, measurement := range result.measurements {
		scorer.noteCandidate(measurement)
	}

	for _, feedback := range result.feedback {
		scorer.applyFeedback(feedback)
	}
}

func (scorer *Scorer) noteCandidate(measurement engine.Measurement) {
	for _, pair := range measurement.Pairs {
		symbol := asset.Symbol(pair)

		if symbol == "" {
			continue
		}

		forecast, ok := BuildSignalForecast(
			measurement, scorer.prices, symbol, scorer.returnModel,
		)

		if !ok {
			continue
		}

		scorer.executable.Note(SignalCandidate{
			Symbol:         symbol,
			Source:         measurement.Source,
			Regime:         measurement.Regime,
			Reason:         measurement.Reason,
			Confidence:     measurement.Confidence,
			ExpectedReturn: forecast.ExpectedReturn,
			Runway:         forecast.Runway,
			Direction:      measurement.Type.Direction(),
			Executable:     true,
		})
	}
}

func (scorer *Scorer) settleDuePredictions(now time.Time) {
	if scorer.prices == nil {
		return
	}

	scorer.pairStates.Range(func(_, value any) bool {
		state, ok := value.(*PairState)

		if !ok || state == nil {
			return true
		}

		symbol := state.Symbol()

		if symbol == "" {
			return true
		}

		quote, ok := scorer.quotePrice(symbol)

		if !ok || quote <= 0 {
			return true
		}

		for _, feedback := range state.SettleDue(now, quote) {
			scorer.applyFeedback(feedback)
		}

		return true
	})
}

func (scorer *Scorer) mergeLiveCandidates() {
	scorer.liveCandidates.Reset()

	for _, signal := range scorer.signals {
		reader, ok := signal.(engine.LiveScoreReader)

		if !ok {
			continue
		}

		peak := reader.PeakReading()

		if peak.Score <= 0 || peak.Symbol == "" {
			continue
		}

		scorer.liveCandidates.Note(SignalCandidate{
			Symbol:     peak.Symbol,
			Source:     signal.Source(),
			Regime:     "live",
			Reason:     "track",
			Confidence: peak.Score,
			Direction:  1,
			Executable: false,
		})
	}
}

func (scorer *Scorer) applyFeedback(feedback engine.PredictionFeedback) {
	if scorer.feedbackSink != nil {
		scorer.feedbackSink(feedback)
	}

	if scorer.returnModel != nil {
		scorer.returnModel.Apply(feedback)
	}

	if scorer.sourceTrust != nil {
		scorer.sourceTrust.Apply(feedback)
	}

	for _, signal := range scorer.signals {
		signal.Feedback(feedback)
	}
}

func (scorer *Scorer) publishCandidateFrame(tickResult signalTickResult) {
	if scorer.candidates == nil {
		return
	}

	warming := scorer.rescoreCount < config.System.MinWarmPulses
	scorer.rescoreCount++

	scorer.candidates.Send(&qpool.QValue[any]{
		Value: CandidateFrame{
			Executable:       scorer.executable.Snapshot(),
			Live:             scorer.liveCandidates.Snapshot(),
			Ready:            !warming,
			PulseSeq:         scorer.rescoreCount,
			TrustWeights:     scorer.sourceTrust.SnapshotWeights(),
			MeasurementCount: len(tickResult.measurements),
		},
	})
}

func (scorer *Scorer) publishEnginePulse(tickResult signalTickResult) {
	if scorer.uiBroadcast == nil {
		return
	}

	forecast := scorer.resolveForecast(nil, 0, nil)

	ui.Publish(scorer.uiBroadcast, "engine_pulse", map[string]any{
		"seq":              scorer.rescoreCount,
		"phase":            "scan",
		"measurements":     len(tickResult.measurements),
		"candidates":       scorer.executable.Len(),
		"open":             0,
		"avg_prediction":   forecast.AvgPrediction,
		"avg_error":        forecast.AvgError,
		"forecast_symbols": forecast.PredictedSymbols,
		"forecast_errors":  forecast.ErrorSymbols,
		"signals":          scorer.signalRows(tickResult.measurements),
		"ticker_ready":     scorer.tickerReady(),
		"symbols_total":    scorer.symbolTotal(),
		"fluid_sampled":    scorer.fluidSampled(),
		"fluid_warming":    scorer.fluidWarming(),
		"forecast_rejects": scorer.forecastRejectSnapshot(),
	})

	if scorer.rescoreCount%50 == 0 {
		errnie.Info(fmt.Sprintf(
			"engine_pulse seq=%d measurements=%d candidates=%d fluid_sampled=%d fluid_warming=%d",
			scorer.rescoreCount,
			len(tickResult.measurements),
			scorer.executable.Len(),
			scorer.fluidSampled(),
			scorer.fluidWarming(),
		))
	}
}

func (scorer *Scorer) signalRows(
	measurements []engine.Measurement,
) []map[string]any {
	rows := make([]map[string]any, 0, len(measurements))

	for _, measurement := range measurements {
		symbol := ""

		if len(measurement.Pairs) > 0 {
			symbol = asset.Symbol(measurement.Pairs[0])
		}

		expectedReturn := 0.0

		if forecast, ok := BuildSignalForecast(
			measurement, scorer.prices, symbol, scorer.returnModel,
		); ok {
			expectedReturn = forecast.ExpectedReturn
		}

		rows = append(rows, map[string]any{
			"symbol":          symbol,
			"source":          measurement.Source,
			"regime":          measurement.Regime,
			"reason":          measurement.Reason,
			"score":           measurement.Confidence,
			"expected_return": expectedReturn,
			"type":            measurement.Type.String(),
		})
	}

	return rows
}

func (scorer *Scorer) resetForecastRejects() {
	scorer.forecastReject = make(map[string]int)
}

func (scorer *Scorer) recordForecastReject(source, reason string) {
	if source == "" || reason == "" {
		return
	}

	if scorer.forecastReject == nil {
		scorer.forecastReject = make(map[string]int)
	}

	scorer.forecastReject[source+":"+reason]++
}

func (scorer *Scorer) forecastRejectSnapshot() map[string]int {
	if len(scorer.forecastReject) == 0 {
		return nil
	}

	snapshot := make(map[string]int, len(scorer.forecastReject))

	for key, value := range scorer.forecastReject {
		snapshot[key] = value
	}

	return snapshot
}

func (scorer *Scorer) pairState(pair asset.Pair) *PairState {
	symbol := asset.Symbol(pair)

	if symbol == "" {
		return nil
	}

	if loaded, ok := scorer.pairStates.Load(symbol); ok {
		return loaded.(*PairState)
	}

	state := NewPairState(pair)
	loaded, _ := scorer.pairStates.LoadOrStore(symbol, state)

	return loaded.(*PairState)
}

func (scorer *Scorer) quotePrice(symbol string) (float64, bool) {
	if scorer.prices == nil || symbol == "" {
		return 0, false
	}

	return scorer.prices.Last(symbol)
}

func (scorer *Scorer) tickerReady() int {
	quotes, ok := scorer.prices.(*MarketQuotes)

	if !ok || quotes == nil {
		return 0
	}

	return quotes.TickerReady()
}

func (scorer *Scorer) symbolTotal() int {
	quotes, ok := scorer.prices.(*MarketQuotes)

	if !ok || quotes == nil {
		return 0
	}

	return quotes.SymbolTotal()
}

func (scorer *Scorer) fluidSampled() int {
	for _, signal := range scorer.signals {
		reader, ok := signal.(interface{ SampledCount() int })

		if ok {
			return reader.SampledCount()
		}
	}

	return 0
}

func (scorer *Scorer) fluidWarming() int {
	for _, signal := range scorer.signals {
		reader, ok := signal.(interface{ WarmingCount() int })

		if ok {
			return reader.WarmingCount()
		}
	}

	return 0
}
