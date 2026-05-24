package trader

import (
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
)

/*
FluidTelemetry exposes fluid field sampling counters for engine_pulse.
*/
type FluidTelemetry interface {
	SampledCount() int
	WarmingCount() int
}

/*
TickerTelemetry exposes quote readiness counters for engine_pulse.
*/
type TickerTelemetry interface {
	ReadyCount() int
}

/*
TelemetryStream publishes dashboard websocket events.
*/
type TelemetryStream interface {
	EnginePulse(payload map[string]any)
	DecisionTrace(payload map[string]any)
	Scoreboard(line, median, mad float64, targets []map[string]any)
	Status(payload map[string]any)
}

/*
Telemetry publishes dashboard events from one trader rescore tick.
*/
type Telemetry struct {
	stream       TelemetryStream
	ticker       TickerTelemetry
	fluid        FluidTelemetry
	symbolsTotal int
	seq          int64
	warmPulses   int

	pulseSignals []map[string]any
	readings     map[string]symbolReadings
}

type signalReading struct {
	source         string
	regime         string
	reason         string
	confidence     float64
	expectedReturn float64
	direction      int
}

type symbolReadings map[string]signalReading

/*
BindTelemetry wires the dashboard publisher for live websocket telemetry.
*/
func (crypto *Crypto) BindTelemetry(
	stream TelemetryStream,
	ticker TickerTelemetry,
	fluid FluidTelemetry,
	symbolsTotal int,
) {
	crypto.telemetry = &Telemetry{
		stream:       stream,
		ticker:       ticker,
		fluid:        fluid,
		symbolsTotal: symbolsTotal,
		readings:     make(map[string]symbolReadings),
	}

	if crypto.portfolio != nil {
		if lifecycle, ok := stream.(PortfolioStream); ok {
			crypto.portfolio.BindStream(lifecycle)
		}
	}
}

/*
BeginTick resets per-cycle telemetry buffers.
*/
func (telemetry *Telemetry) BeginTick() {
	if telemetry == nil || telemetry.stream == nil {
		return
	}

	telemetry.seq++
	telemetry.pulseSignals = telemetry.pulseSignals[:0]
	telemetry.readings = make(map[string]symbolReadings)
}

/*
NoteMeasurement records one drained signal reading for pulse and forecast rows.
*/
func (telemetry *Telemetry) NoteMeasurement(measurement engine.Measurement) {
	if telemetry == nil || telemetry.stream == nil || measurement.Confidence <= 0 {
		return
	}

	for _, pair := range measurement.Pairs {
		symbol := asset.Symbol(pair)
		if symbol == "" {
			continue
		}

		telemetry.pulseSignals = append(telemetry.pulseSignals, map[string]any{
			"symbol":          symbol,
			"source":          measurement.Source,
			"regime":          measurement.Regime,
			"reason":          measurement.Reason,
			"score":           measurement.Confidence,
			"expected_return": measurement.ExpectedReturn,
			"type":            measurementTypeName(measurement.Type),
		})

		if telemetry.readings[symbol] == nil {
			telemetry.readings[symbol] = make(symbolReadings)
		}

		existing := telemetry.readings[symbol][measurement.Source]

		if existing.confidence >= measurement.Confidence &&
			existing.expectedReturn >= measurement.ExpectedReturn {
			continue
		}

		if measurement.Confidence > existing.confidence {
			existing.confidence = measurement.Confidence
			existing.regime = measurement.Regime
			existing.reason = measurement.Reason
		}

		if measurement.ExpectedReturn > existing.expectedReturn {
			existing.expectedReturn = measurement.ExpectedReturn
		}

		existing.direction = measurement.Type.Direction()
		existing.source = measurement.Source
		telemetry.readings[symbol][measurement.Source] = existing
	}
}

/*
Publish emits status, engine_pulse, decision_trace, and scoreboard for the tick.
Decision rows come from the trader decision snapshot, not a separate gate path.
*/
func (telemetry *Telemetry) Publish(wallet *Wallet, crypto *Crypto) {
	if telemetry == nil || telemetry.stream == nil {
		return
	}

	if telemetry.seq > 0 {
		telemetry.warmPulses++
	}

	phase := "scan"

	if telemetry.warmPulses < config.System.MinWarmPulses {
		phase = "warming"
	}

	if crypto != nil {
		telemetry.ingestLiveReadings(crypto)
	}

	snapshot := DecisionSnapshot{}

	if crypto != nil {
		snapshot = crypto.DecisionSnapshot()
	}

	evaluations := evaluationsToMaps(snapshot.Evaluations)
	decisions := decisionsToMaps(snapshot.Decisions)
	line := snapshot.Line
	median := snapshot.Median
	mad := snapshot.MAD
	targets := scoreboardTargetsFromEvaluations(snapshot.Evaluations, line)

	forecast := ForecastSnapshot{}

	if crypto != nil {
		forecast = crypto.resolveForecast(telemetry.readings, line, evaluations)
	}

	sourceScores := map[string]float64{}

	if crypto != nil {
		sourceScores = crypto.sourceScores()
	}

	allowed := 0

	for _, row := range snapshot.Evaluations {
		if row.Allow {
			allowed++
		}
	}

	openCount := 0

	if crypto != nil && crypto.portfolio != nil && crypto.prices != nil {
		openCount = crypto.portfolio.Status(crypto.prices).OpenCount
	}

	telemetry.stream.EnginePulse(map[string]any{
		"seq":              telemetry.seq,
		"phase":            phase,
		"measurements":     len(telemetry.pulseSignals),
		"candidates":       len(evaluations),
		"open":             openCount,
		"ticker_ready":     telemetry.tickerReady(),
		"symbols_total":    telemetry.symbolsTotal,
		"fluid_sampled":    telemetry.fluidSampled(),
		"fluid_warming":    telemetry.fluidWarming(),
		"signals":          telemetry.pulseSignals,
		"source_scores":    sourceScores,
		"avg_prediction":   forecast.AvgPrediction,
		"avg_error":        forecast.AvgError,
		"forecast_symbols": forecast.PredictedSymbols,
		"forecast_errors":  forecast.ErrorSymbols,
	})

	telemetry.stream.DecisionTrace(map[string]any{
		"line":        line,
		"median":      median,
		"mad":         mad,
		"scored":      len(evaluations),
		"in_play":     len(evaluations),
		"allowed":     allowed,
		"decisions":   decisions,
		"evaluations": evaluations,
	})

	telemetry.stream.Scoreboard(line, median, mad, targets)

	status := map[string]any{
		"equity_eur":     walletBalance(wallet),
		"cash_eur":       walletBalance(wallet),
		"closed_pnl_eur": 0,
		"trade_count":    0,
		"win_rate":       0,
		"open_count":     0,
	}

	if crypto != nil && crypto.portfolio != nil && crypto.prices != nil {
		portfolioStatus := crypto.portfolio.Status(crypto.prices)
		status["equity_eur"] = portfolioStatus.EquityEUR
		status["cash_eur"] = portfolioStatus.CashEUR
		status["closed_pnl_eur"] = portfolioStatus.ClosedPnLEUR
		status["trade_count"] = portfolioStatus.TradeCount
		status["win_rate"] = portfolioStatus.WinRate
		status["open_count"] = portfolioStatus.OpenCount

		if len(portfolioStatus.Positions) > 0 {
			status["positions"] = portfolioStatus.Positions
		}
	}

	telemetry.stream.Status(status)
}

func (telemetry *Telemetry) ingestLiveReadings(crypto *Crypto) {
	for _, signal := range crypto.signals {
		reader, ok := signal.(engine.LiveScoreReader)

		if !ok {
			continue
		}

		peak := reader.PeakReading()

		if peak.Score <= 0 {
			continue
		}

		source := signal.Source()

		if peak.Symbol == "" {
			continue
		}

		telemetry.mergeLiveReading(peak.Symbol, source, peak.Score)

		if telemetry.hasPulseSignal(peak.Symbol, source) {
			continue
		}

		telemetry.pulseSignals = append(telemetry.pulseSignals, map[string]any{
			"symbol": peak.Symbol,
			"source": source,
			"regime": "live",
			"reason": "track",
			"score":  peak.Score,
			"type":   "live",
		})
	}
}

func (telemetry *Telemetry) mergeLiveReading(
	symbol, source string,
	score float64,
) {
	if telemetry.readings[symbol] == nil {
		telemetry.readings[symbol] = make(symbolReadings)
	}

	existing := telemetry.readings[symbol][source]

	if score <= existing.confidence {
		return
	}

	existing.confidence = score
	existing.source = source
	existing.regime = "live"
	existing.reason = "track"
	telemetry.readings[symbol][source] = existing
}

func (telemetry *Telemetry) hasPulseSignal(symbol, source string) bool {
	for _, row := range telemetry.pulseSignals {
		rowSymbol, _ := row["symbol"].(string)
		rowSource, _ := row["source"].(string)

		if rowSymbol == symbol && rowSource == source {
			return true
		}
	}

	return false
}

func evaluationsToMaps(evaluations []Evaluation) []map[string]any {
	rows := make([]map[string]any, len(evaluations))

	for index, evaluation := range evaluations {
		rows[index] = evaluationToMap(evaluation)
	}

	return rows
}

func decisionsToMaps(decisions []Decision) []map[string]any {
	rows := make([]map[string]any, len(decisions))

	for index, decision := range decisions {
		rows[index] = decisionToMap(decision)
	}

	return rows
}

func scoreboardTargetsFromEvaluations(
	evaluations []Evaluation,
	line float64,
) []map[string]any {
	targets := make([]map[string]any, 0, len(evaluations))

	for _, row := range evaluations {
		if row.CombinedScore < line {
			continue
		}

		targets = append(targets, map[string]any{
			"symbol":          row.Symbol,
			"regime":          row.Regime,
			"reason":          row.Reason,
			"score":           row.CombinedScore,
			"effective_score": row.CombinedScore,
			"trail_pct":       0,
		})
	}

	return targets
}

func whyCode(warming bool, score, line float64) string {
	if warming {
		return "field_warming"
	}

	if line > 0 && score < line {
		return "below_line"
	}

	if score <= 0 {
		return "below_line"
	}

	return "ok"
}

func measurementTypeName(measurementType engine.MeasurementType) string {
	switch measurementType {
	case engine.Pump:
		return "pump"
	case engine.Dump:
		return "dump"
	case engine.Momentum:
		return "momentum"
	case engine.Flow:
		return "flow"
	case engine.Causal:
		return "causal"
	default:
		return "unknown"
	}
}

func walletBalance(wallet *Wallet) float64 {
	if wallet == nil {
		return 0
	}

	return wallet.Balance
}

func sideLabel(direction int) string {
	if direction < 0 {
		return "short"
	}

	return "long"
}

func (telemetry *Telemetry) tickerReady() int {
	if telemetry.ticker == nil {
		return 0
	}

	return telemetry.ticker.ReadyCount()
}

func (telemetry *Telemetry) fluidSampled() int {
	if telemetry.fluid == nil {
		return 0
	}

	return telemetry.fluid.SampledCount()
}

func (telemetry *Telemetry) fluidWarming() int {
	if telemetry.fluid == nil {
		return 0
	}

	return telemetry.fluid.WarmingCount()
}
