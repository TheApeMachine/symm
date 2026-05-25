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
Crypto trades Kraken microstructure signals with trailing stops.
Paper mode simulates fills; live mode submits orders over WebSocket v2 when API keys are set.
*/
type Crypto struct {
	ctx            context.Context
	cancel         context.CancelFunc
	pool           *qpool.Q
	uiBroadcast    *qpool.BroadcastGroup
	wallet         *Wallet
	portfolio      *Portfolio
	prices         QuoteReader
	market         engine.MarketReader
	signals        []engine.Signal
	tickers        []engine.Ticker
	pairStates     sync.Map
	feedbackSink   func(engine.PredictionFeedback)
	candidates     CandidateStore
	liveCandidates CandidateStore
	decisionEngine DecisionEngine
	sourceTrust    *SourceTrustStore
	returnModel    *ReturnModel
	forecastReject map[string]int
	orderJournal   *OrderJournal
	exitAdvisor    ExitAdvisor
	symbolUniverse []string
	lastDecision   DecisionSnapshot
	rescoreCount   int
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
	market engine.MarketReader,
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
		market:         market,
		signals:        signals,
		pairStates:     sync.Map{},
		candidates:     NewCandidateStore(),
		liveCandidates: NewCandidateStore(),
		decisionEngine: DecisionEngine{},
		sourceTrust:    NewSourceTrustStore(),
		returnModel:    NewReturnModel(),
		forecastReject: make(map[string]int),
		orderJournal:   NewOrderJournal(config.System.LogDir),
	}

	portfolioStore := NewPortfolioStore(config.System.LogDir)

	if err := portfolioStore.Restore(crypto.portfolio); err != nil {
		return nil, err
	}

	crypto.portfolio.BindPortfolioStore(portfolioStore)

	crypto.portfolio.BindRiskReader(NewSignalRiskBoard(signals...))
	crypto.portfolio.BindUI(ui)

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
SetSymbolUniverse pins regime classification to a stable scan set.
*/
func (crypto *Crypto) SetSymbolUniverse(symbols []string) {
	crypto.symbolUniverse = append([]string(nil), symbols...)
}

/*
OrderJournal returns the live order journal when configured.
*/
func (crypto *Crypto) OrderJournal() *OrderJournal {
	return crypto.orderJournal
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
BindExitAdvisor wires book-exhaustion scoring into paper exits.
*/
func (crypto *Crypto) BindExitAdvisor(advisor ExitAdvisor) {
	crypto.exitAdvisor = advisor
	crypto.portfolio.BindExitAdvisor(advisor)
}

/*
BindBroker replaces the default paper broker with a live Kraken execution broker.
*/
func (crypto *Crypto) BindBroker(broker ExecutionBroker) {
	crypto.portfolio.BindBroker(broker)
	crypto.portfolio.BindOrderJournal(crypto.orderJournal)
}

/*
CreditWarmPulses advances the warm-up counter after OHLC bootstrap.
*/
func (crypto *Crypto) CreditWarmPulses(credit int) {
	if credit <= 0 {
		return
	}

	crypto.rescoreCount += credit
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

	rescoreTicker := time.NewTicker(config.System.RescoreEvery)
	defer rescoreTicker.Stop()

	for {
		select {
		case <-crypto.ctx.Done():
			return crypto.ctx.Err()
		case <-rescoreTicker.C:
			crypto.runRescoreTick()
		}
	}
}

func (crypto *Crypto) runRescoreTick() {
	now := time.Now().UTC()
	crypto.drainTickables()
	crypto.candidates.Reset()
	crypto.resetForecastRejects()

	tickResult := crypto.processSignals(now)

	crypto.drainTickables()
	crypto.settleDuePredictions(now)
	crypto.drainTickables()
	crypto.runExecution(now)
	crypto.publishEnginePulse(tickResult)
}

func (crypto *Crypto) drainTickables() {
	if len(crypto.tickers) == 0 {
		return
	}

	perTickerLimit := config.System.MaxPendingPerSignal

	if perTickerLimit <= 0 {
		errnie.Error(fmt.Errorf("max pending per signal must be positive, got %d", perTickerLimit))

		return
	}

	remaining := crypto.tickDrainGlobalLimit(perTickerLimit)

	for _, ticker := range crypto.tickers {
		if remaining <= 0 {
			return
		}

		drainLimit := min(perTickerLimit, remaining)
		remaining -= crypto.drainTicker(ticker, drainLimit)
	}
}

func (crypto *Crypto) tickDrainGlobalLimit(perTickerLimit int) int {
	globalLimit := config.System.MaxPendingGlobal

	if globalLimit < 0 {
		errnie.Error(fmt.Errorf("max pending global must be non-negative, got %d", globalLimit))

		return 0
	}

	if globalLimit > 0 {
		return globalLimit
	}

	return perTickerLimit * len(crypto.tickers)
}

func (crypto *Crypto) drainTicker(ticker engine.Ticker, limit int) int {
	if limit <= 0 {
		return 0
	}

	drainTicker, ok := ticker.(engine.DrainTicker)

	if ok {
		return drainTicker.Drain(limit)
	}

	drained := 0

	for drained < limit {
		if !ticker.Tick() {
			return drained
		}

		drained++
	}

	return drained
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

	if reader, ok := signal.(engine.MeanConfidenceReader); ok && crypto.uiBroadcast != nil {
		ui.Publish(crypto.uiBroadcast, "signal_score", map[string]any{
			"source":     signal.Source(),
			"confidence": reader.MeanConfidence(),
		})
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

		forecast, reason := BuildSignalForecastReason(
			measurement, crypto.prices, symbol, crypto.returnModel,
		)

		if reason != "" {
			crypto.recordForecastReject(measurement.Source, reason)
			crypto.recordCalibrationProbe(state, measurement, now, symbol, reason)
			continue
		}

		state.ApplyForecast(forecast)

		baselineQuote, ok := crypto.quotePrice(symbol)

		if !ok {
			continue
		}

		state.RecordPrediction(now, measurement, forecast, baselineQuote)
		pending = append(pending, state.SettleDue(now, baselineQuote)...)
	}

	return pending
}

func (crypto *Crypto) processSignal(signal engine.Signal, now time.Time) error {
	crypto.applyTickResult(crypto.collectMeasurements(signal, now))

	return nil
}

func (crypto *Crypto) publishEnginePulse(tickResult signalTickResult) {
	if crypto.uiBroadcast == nil {
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

	ui.Publish(crypto.uiBroadcast, "engine_pulse", map[string]any{
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
		"forecast_rejects": crypto.forecastRejectSnapshot(),
	})

	if crypto.rescoreCount%50 == 0 {
		errnie.Info(fmt.Sprintf(
			"engine_pulse seq=%d measurements=%d candidates=%d fluid_sampled=%d fluid_warming=%d",
			crypto.rescoreCount,
			len(tickResult.measurements),
			crypto.candidates.Len(),
			crypto.fluidSampled(),
			crypto.fluidWarming(),
		))
	}
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

		if forecast, ok := BuildSignalForecast(
			measurement, crypto.prices, symbol, crypto.returnModel,
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

func (crypto *Crypto) publishDashboard() {
	if crypto.uiBroadcast == nil {
		return
	}

	crypto.publishStatus()

	targets := scoreboardTargets(
		crypto.lastDecision,
		crypto.prices,
		crypto.portfolioRiskReader(),
	)
	ui.Publish(crypto.uiBroadcast, "scoreboard", map[string]any{
		"line":    crypto.lastDecision.Line,
		"median":  crypto.lastDecision.Median,
		"mad":     crypto.lastDecision.MAD,
		"targets": targets,
	})

	ui.Publish(
		crypto.uiBroadcast,
		"decision_trace",
		decisionTracePayload(crypto.lastDecision, crypto.candidates, crypto.liveCandidates),
	)
}

func (crypto *Crypto) publishStatus() {
	if crypto.uiBroadcast == nil || crypto.portfolio == nil {
		return
	}

	ui.Publish(crypto.uiBroadcast, "status", statusPayload(crypto.portfolio.Status(crypto.prices)))
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
	crypto.resetForecastRejects()

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

		forecast, ok := BuildSignalForecast(
			measurement, crypto.prices, symbol, crypto.returnModel,
		)

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
			Executable:     true,
		})
	}
}

func (crypto *Crypto) resetForecastRejects() {
	crypto.forecastReject = make(map[string]int)
}

func (crypto *Crypto) recordForecastReject(source, reason string) {
	if source == "" || reason == "" {
		return
	}

	if crypto.forecastReject == nil {
		crypto.forecastReject = make(map[string]int)
	}

	crypto.forecastReject[source+":"+reason]++
}

func (crypto *Crypto) forecastRejectSnapshot() map[string]int {
	if len(crypto.forecastReject) == 0 {
		return nil
	}

	snapshot := make(map[string]int, len(crypto.forecastReject))

	for key, value := range crypto.forecastReject {
		snapshot[key] = value
	}

	return snapshot
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
	crypto.liveCandidates.Reset()

	for _, signal := range crypto.signals {
		reader, ok := signal.(engine.LiveScoreReader)

		if !ok {
			continue
		}

		peak := reader.PeakReading()

		if peak.Score <= 0 || peak.Symbol == "" {
			continue
		}

		crypto.liveCandidates.Note(SignalCandidate{
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

	if crypto.returnModel != nil {
		crypto.returnModel.Apply(feedback)
	}

	if crypto.sourceTrust != nil {
		crypto.sourceTrust.Apply(feedback)
	}

	for _, signal := range crypto.signals {
		signal.Feedback(feedback)
	}
}

func (crypto *Crypto) runExecution(now time.Time) {
	if crypto.portfolio == nil || crypto.prices == nil {
		return
	}

	for _, event := range crypto.portfolio.Mark(crypto.ctx, now, crypto.prices) {
		markEvent := event
		crypto.portfolio.Emit(&markEvent)
		crypto.handlePortfolioEvent(markEvent)
	}

	crypto.publishStatus()

	crypto.mergeLiveCandidates()

	warming := crypto.rescoreCount < config.System.MinWarmPulses
	crypto.rescoreCount++
	regime := ClassifyMarketRegime(crypto.market, crypto.regimeSymbols(), now)
	cashEUR := config.System.WalletEUR

	if crypto.wallet != nil {
		cashEUR = crypto.wallet.Balance
	}

	crypto.lastDecision = crypto.decisionEngine.Build(
		crypto.candidates,
		crypto.prices,
		crypto.market,
		now,
		cashEUR,
		warming,
		EnsembleContext{
			Regime: regime,
			Trust:  crypto.sourceTrust,
		},
	)

	crypto.publishDashboard()

	if warming {
		return
	}

	if !crypto.portfolio.TradingAllowed() {
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

		if crypto.market != nil {
			snapshot := crypto.market.ReadFresh(
				evaluation.Symbol,
				now,
				config.System.SnapshotFreshnessTTL,
			)

			if !snapshot.LastOK || !snapshot.SpreadOK || !snapshot.BatchOK {
				continue
			}
		}

		if event, ok := crypto.portfolio.TryEnter(crypto.ctx, now, decision, crypto.prices); ok {
			crypto.portfolio.Emit(event)
			crypto.watchExitSymbol(evaluation.Symbol)
			crypto.publishStatus()
		}
	}
}

func (crypto *Crypto) handlePortfolioEvent(event PortfolioEvent) {
	if event.Name != "trade_exit" {
		return
	}

	symbol, _ := event.Payload["symbol"].(string)
	crypto.forgetExitSymbol(symbol)
}

func (crypto *Crypto) watchExitSymbol(symbol string) {
	watcher, ok := crypto.exitAdvisor.(interface {
		WatchSymbol(string)
	})

	if !ok || symbol == "" {
		return
	}

	watcher.WatchSymbol(symbol)
}

func (crypto *Crypto) forgetExitSymbol(symbol string) {
	watcher, ok := crypto.exitAdvisor.(interface {
		ForgetSymbol(string)
	})

	if !ok || symbol == "" {
		return
	}

	watcher.ForgetSymbol(symbol)
}

func (crypto *Crypto) regimeSymbols() []string {
	if len(crypto.symbolUniverse) > 0 {
		budget := config.System.MaxScanSymbols

		if budget <= 0 || budget >= len(crypto.symbolUniverse) {
			return crypto.symbolUniverse
		}

		return crypto.symbolUniverse[:budget]
	}

	return crypto.candidates.Symbols()
}
