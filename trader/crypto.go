package trader

import (
	"context"
	"sync"
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
	uiBroadcast    *qpool.BroadcastGroup
	wallet         *Wallet
	portfolio      *Portfolio
	prices         QuoteReader
	signals        []engine.Signal
	tickers        []engine.Ticker
	pairStates     sync.Map
	feedbackSink   func(engine.PredictionFeedback)
	candidates     CandidateStore
	decisionEngine DecisionEngine
	lastDecision   DecisionSnapshot
	rescoreCount   int
	uiStream       UIStream
}

/*
NewCrypto creates a new crypto trader.
*/
func NewCrypto(
	ctx context.Context,
	pool *qpool.Q,
	ui *qpool.BroadcastGroup,
	wallet *Wallet,
	prices QuoteReader,
	signals ...engine.Signal,
) (*Crypto, error) {
	ctx, cancel := context.WithCancel(ctx)

	if ui == nil && pool != nil {
		ui = pool.CreateBroadcastGroup("ui", 10*time.Millisecond)
	}

	crypto := &Crypto{
		ctx:            ctx,
		cancel:         cancel,
		pool:           pool,
		uiBroadcast:    ui,
		wallet:         wallet,
		portfolio:      NewPortfolio(wallet),
		prices:         prices,
		signals:        signals,
		pairStates:     sync.Map{},
		candidates:     NewCandidateStore(),
		decisionEngine: DecisionEngine{},
	}

	crypto.portfolio.BindRiskReader(NewSignalRiskBoard(signals...))

	for _, signal := range signals {
		ticker, ok := signal.(engine.Ticker)

		if !ok {
			continue
		}

		crypto.tickers = append(crypto.tickers, ticker)
	}

	return crypto, errnie.Require(map[string]any{
		"ctx":        ctx,
		"cancel":     cancel,
		"pool":       pool,
		"wallet":     wallet,
		"prices":     prices,
		"signals":    signals,
		"pairStates": &crypto.pairStates,
	})
}

/*
BindUIStream attaches the dashboard websocket publisher.
*/
func (crypto *Crypto) BindUIStream(stream UIStream) {
	crypto.uiStream = stream
}

/*
BindPortfolioStream wires trade lifecycle events to the dashboard stream.
*/
func (crypto *Crypto) BindPortfolioStream(stream PortfolioStream) {
	if crypto.portfolio == nil {
		return
	}

	crypto.portfolio.BindStream(stream)
}

/*
Bootstrap is retained for tests; live clients receive incremental events only.
*/
func (crypto *Crypto) Bootstrap() []map[string]any {
	return nil
}

/*
PrimeDashboard publishes wallet status before the run loop starts.
*/
func (crypto *Crypto) PrimeDashboard() {
	crypto.publishStatus()
}

/*
RegisterTicker adds one orchestrator-driven message consumer.
*/
func (crypto *Crypto) RegisterTicker(ticker engine.Ticker) {
	if ticker == nil {
		return
	}

	crypto.tickers = append(crypto.tickers, ticker)
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
DecisionSnapshot returns the latest decision snapshot for telemetry.
*/
func (crypto *Crypto) DecisionSnapshot() DecisionSnapshot {
	return crypto.lastDecision
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
Run runs the crypto trader on a single scheduler: tick → measure → decide.
*/
func (crypto *Crypto) Run() error {
	crypto.uiBroadcast.Send(&qpool.QValue[any]{
		Value: map[string]any{
			"type":   "rescore_begin",
			"ts":     time.Now().UTC().Format(time.RFC3339Nano),
			"wallet": crypto.wallet,
		},
	})

	crypto.publishDashboard()

	for {
		select {
		case <-crypto.ctx.Done():
			return crypto.ctx.Err()
		default:
			now := time.Now().UTC()
			crypto.drainTickables()
			crypto.candidates.Reset()

			tickResult := crypto.processSignals(now)

			crypto.drainTickables()
			crypto.settleDuePredictions(now)
			crypto.drainTickables()
			crypto.runExecution(now)
			crypto.publishEnginePulse(tickResult)
		}
	}
}

func (crypto *Crypto) drainTickables() {
	for {
		idle := true

		for _, ticker := range crypto.tickers {
			if ticker.Tick() {
				idle = false
			}
		}

		if idle {
			return
		}
	}
}

type signalTickResult struct {
	measurements []engine.Measurement
	feedback     []engine.PredictionFeedback
}

/*
processSignals measures every signal once per rescore tick on the orchestrator thread.
Per-symbol evaluate steps run on the injected qpool while the orchestrator drains
tickables between worker completions.
*/
func (crypto *Crypto) measureContext() context.Context {
	return engine.WithTickDrain(crypto.ctx, crypto.drainTickables)
}

func (crypto *Crypto) processSignals(now time.Time) signalTickResult {
	measureCtx := crypto.measureContext()
	result := signalTickResult{}

	for _, signal := range crypto.signals {
		engine.DrainTicks(measureCtx)
		tickResult := crypto.collectMeasurements(signal, now)
		crypto.applyTickResult(tickResult)
		result.measurements = append(result.measurements, tickResult.measurements...)
		result.feedback = append(result.feedback, tickResult.feedback...)
	}

	return result
}

func (crypto *Crypto) collectMeasurements(
	signal engine.Signal, now time.Time,
) signalTickResult {
	result := signalTickResult{}

	for measurement := range signal.Measure(crypto.measureContext(), now) {
		result.measurements = append(result.measurements, measurement)
		result.feedback = append(
			result.feedback,
			crypto.updatePairStates(measurement, now)...,
		)
	}

	if reader, ok := signal.(engine.MeanConfidenceReader); ok && crypto.uiStream != nil {
		crypto.uiStream.SignalScore(signal.Source(), reader.MeanConfidence())
	}

	return result
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

		symbol := asset.Symbol(pair)

		state.Update(measurement)

		forecast, ok := BuildSignalForecast(measurement, crypto.prices, symbol)

		if !ok {
			continue
		}

		state.ApplyForecast(forecast)
		state.RecordPrediction(now, measurement, forecast)

		quotePrice, ok := crypto.quotePrice(symbol)

		if !ok {
			continue
		}

		state.AnchorPending(quotePrice)
		pending = append(pending, state.SettleDue(now, quotePrice)...)
	}

	return pending
}

func (crypto *Crypto) processSignal(signal engine.Signal, now time.Time) error {
	crypto.applyTickResult(crypto.collectMeasurements(signal, now))

	return nil
}

func (crypto *Crypto) publishEnginePulse(tickResult signalTickResult) {
	if crypto.uiStream == nil {
		return
	}

	openCount := 0

	if crypto.portfolio != nil && crypto.prices != nil {
		openCount = crypto.portfolio.Status(crypto.prices).OpenCount
	}

	forecast := crypto.resolveForecast(
		nil,
		crypto.lastDecision.Line,
		crypto.evaluationRows(),
	)

	crypto.uiStream.EnginePulse(map[string]any{
		"seq":              crypto.rescoreCount,
		"phase":            "scan",
		"measurements":     len(tickResult.measurements),
		"candidates":       crypto.candidates.Len(),
		"open":             openCount,
		"avg_prediction":   forecast.AvgPrediction,
		"avg_error":        forecast.AvgError,
		"forecast_symbols": forecast.PredictedSymbols,
		"forecast_errors":  forecast.ErrorSymbols,
		"signals":          crypto.signalRows(tickResult.measurements),
		"ticker_ready":     crypto.tickerReady(),
		"symbols_total":    crypto.symbolTotal(),
		"fluid_sampled":    crypto.fluidSampled(),
		"fluid_warming":    crypto.fluidWarming(),
	})
}

func (crypto *Crypto) signalRows(
	measurements []engine.Measurement,
) []map[string]any {
	rows := make([]map[string]any, 0, len(measurements))

	for _, measurement := range measurements {
		symbol := ""

		if len(measurement.Pairs) > 0 {
			symbol = asset.Symbol(measurement.Pairs[0])
		}

		expectedReturn := 0.0

		if forecast, ok := BuildSignalForecast(measurement, crypto.prices, symbol); ok {
			expectedReturn = forecast.ExpectedReturn
		}

		rows = append(rows, map[string]any{
			"symbol":          symbol,
			"source":          measurement.Source,
			"regime":          measurement.Regime,
			"reason":          measurement.Reason,
			"score":           measurement.Confidence,
			"expected_return": expectedReturn,
			"type":            string(measurement.Type),
		})
	}

	return rows
}

func (crypto *Crypto) publishDashboard() {
	if crypto.uiStream == nil {
		return
	}

	crypto.publishStatus()

	targets := scoreboardTargets(
		crypto.lastDecision,
		crypto.prices,
		crypto.portfolioRiskReader(),
	)
	crypto.uiStream.Scoreboard(
		crypto.lastDecision.Line,
		crypto.lastDecision.Median,
		crypto.lastDecision.MAD,
		targets,
	)

	tracePayload := decisionTracePayload(crypto.lastDecision, crypto.candidates)
	tracePayload["event"] = "decision_trace"
	crypto.uiStream.DecisionTrace(tracePayload)
}

func (crypto *Crypto) publishStatus() {
	if crypto.uiStream == nil || crypto.portfolio == nil {
		return
	}

	statusPayload := statusPayload(crypto.portfolio.Status(crypto.prices))
	statusPayload["event"] = "status"
	crypto.uiStream.Status(statusPayload)
}

func (crypto *Crypto) tickerReady() int {
	quotes, ok := crypto.prices.(*MarketQuotes)

	if !ok || quotes == nil {
		return 0
	}

	return quotes.TickerReady()
}

func (crypto *Crypto) symbolTotal() int {
	quotes, ok := crypto.prices.(*MarketQuotes)

	if !ok || quotes == nil {
		return 0
	}

	return quotes.SymbolTotal()
}

func (crypto *Crypto) fluidSampled() int {
	for _, signal := range crypto.signals {
		reader, ok := signal.(interface{ SampledCount() int })

		if ok {
			return reader.SampledCount()
		}
	}

	return 0
}

func (crypto *Crypto) fluidWarming() int {
	for _, signal := range crypto.signals {
		reader, ok := signal.(interface{ WarmingCount() int })

		if ok {
			return reader.WarmingCount()
		}
	}

	return 0
}

func (crypto *Crypto) portfolioRiskReader() RiskReader {
	if crypto.portfolio == nil {
		return nil
	}

	return crypto.portfolio.riskReader
}

func (crypto *Crypto) evaluationRows() []map[string]any {
	rows := make([]map[string]any, 0, len(crypto.lastDecision.Evaluations))

	for _, evaluation := range crypto.lastDecision.Evaluations {
		rows = append(rows, map[string]any{
			"expected_return": evaluation.ExpectedReturn,
			"combined":        evaluation.CombinedScore,
		})
	}

	return rows
}

/*
Rescore runs one full tick: measure signals, settle predictions, and execute.
*/
func (crypto *Crypto) Rescore(now time.Time) error {
	crypto.drainTickables()
	crypto.candidates.Reset()

	tickResult := crypto.processSignals(now)
	crypto.settleDuePredictions(now)
	crypto.runExecution(now)
	crypto.publishEnginePulse(tickResult)

	return nil
}

func (crypto *Crypto) applyTickResult(result signalTickResult) {
	for _, measurement := range result.measurements {
		crypto.noteCandidate(measurement)
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

		forecast, ok := BuildSignalForecast(measurement, crypto.prices, symbol)

		if !ok {
			continue
		}

		crypto.candidates.Note(SignalCandidate{
			Symbol:         symbol,
			Source:         measurement.Source,
			Regime:         measurement.Regime,
			Reason:         measurement.Reason,
			Confidence:     measurement.Confidence,
			ExpectedReturn: forecast.ExpectedReturn,
			Runway:         forecast.Runway,
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
		signal.Feedback(feedback)
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

	crypto.publishStatus()

	crypto.mergeLiveCandidates()

	warming := crypto.rescoreCount < config.System.MinWarmPulses
	crypto.rescoreCount++
	crypto.lastDecision = crypto.decisionEngine.Build(
		crypto.candidates, crypto.prices, warming,
	)

	crypto.publishDashboard()

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
			crypto.publishStatus()
		}
	}
}
