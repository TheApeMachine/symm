package trader

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/user"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/signal/causal"
	"github.com/theapemachine/symm/signal/correlation"
	"github.com/theapemachine/symm/signal/cvd"
	"github.com/theapemachine/symm/signal/depthflow"
	"github.com/theapemachine/symm/signal/exhaust"
	"github.com/theapemachine/symm/signal/fluid"
	"github.com/theapemachine/symm/signal/hawkes"
	"github.com/theapemachine/symm/signal/leadlag"
	"github.com/theapemachine/symm/signal/liquidity"
	"github.com/theapemachine/symm/signal/manifold"
	"github.com/theapemachine/symm/signal/pumpdump"
	"github.com/theapemachine/symm/signal/replay"
	"github.com/theapemachine/symm/signal/resonance"
	"github.com/theapemachine/symm/signal/sentiment"
	"github.com/theapemachine/symm/signal/toxicity"
	"github.com/theapemachine/symm/trader/cognitive"
	"github.com/theapemachine/symm/ui"
)

/*
surpriseStats tracks running mean and variance for resonance surprise gating.
*/
type surpriseStats struct {
	mean float64
	m2   float64
	n    float64
}

/*
Crypto is a trader that is responsible for orchestrating
the trading of crypto assets. It should collect the data
it needs to make informed decisions regarding the opening,
closing, and reporting of positions.
*/
type Crypto struct {
	ctx               context.Context
	cancel            context.CancelFunc
	pool              *qpool.Q[any]
	broadcasts        *sync.Map
	subscribers       *sync.Map
	instrument        *Instrument
	book              *Book
	ticker            *Ticker
	trade             *Trade
	ohlc              *OHLC
	action            *Action
	execution         *Execution
	balances          *Balances
	causalSignal      *causal.Signal
	correlationSignal *correlation.Signal
	cvdSignal         *cvd.Signal
	depthflowSignal   *depthflow.Signal
	exhaustSignal     *exhaust.Signal
	fluidSignal       *fluid.Signal
	hawkesSignal      *hawkes.Signal
	leadlagSignal     *leadlag.Signal
	liquiditySignal   *liquidity.Signal
	manifoldSignal    *manifold.Signal
	cognitiveMemory   *cognitive.Memory
	pumpdumpSignal    *pumpdump.Signal
	resonanceSignal   *resonance.Signal
	sentimentSignal   *sentiment.Signal
	toxicitySignal    *toxicity.Signal
	scopes            *sync.Map
	story             *market.Story
	balancesSub       sync.Once
	resonanceSurprise *sync.Map
	surpriseThreshold float64
	storyTicks        atomic.Uint64
}

func NewCrypto(
	ctx context.Context, pool *qpool.Q[any],
) *Crypto {
	ctx, cancel := context.WithCancel(ctx)

	crypto := &Crypto{
		ctx:               ctx,
		cancel:            cancel,
		pool:              pool,
		broadcasts:        &sync.Map{},
		subscribers:       &sync.Map{},
		instrument:        NewInstrument(ctx, pool),
		book:              NewBook(ctx),
		ticker:            NewTicker(ctx),
		trade:             NewTrade(ctx),
		ohlc:              NewOHLC(ctx),
		action:            NewAction(ctx),
		execution:         NewExecution(ctx),
		balances:          NewBalances(ctx, pool),
		scopes:            &sync.Map{},
		story:             market.NewStory(ctx, pool),
		resonanceSurprise: &sync.Map{},
		surpriseThreshold: viper.GetFloat64("signals.resonance.surprise_threshold"),
	}

	if crypto.surpriseThreshold <= 0 {
		crypto.surpriseThreshold = 1.5
	}

	crypto.initSignals()
	crypto.cognitiveMemory = cognitive.NewMemory(crypto.ctx)

	for _, channel := range []string{
		"desk", "ui",
	} {
		crypto.broadcasts.Store(
			channel, pool.CreateBroadcastGroup(channel),
		)
	}

	for _, channel := range []string{
		"ticker", "book", "trade", "ohlc", "instrument", "action", "execution", "balances", "status",
	} {
		crypto.subscribers.Store(
			channel, pool.Subscribe(channel, crypto.onMessage),
		)
	}

	return crypto
}

func (crypto *Crypto) Run() error {
	errnie.Error(crypto.balances.Subscribe())
	crypto.resubscribePublic()

	interval := viper.GetDuration("market.story.ui_interval")

	if interval <= 0 {
		interval = time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-crypto.ctx.Done():
			return nil
		case <-ticker.C:
			crypto.measure()

			for _, action := range crypto.story.Actions() {
				if action == nil {
					continue
				}

				if !crypto.applyCognitiveAction(action) {
					continue
				}

				crypto.action.Update(*action)
			}
		}
	}
}

func (crypto *Crypto) onMessage(artifact *datura.Artifact) error {
	switch datura.Peek[string](artifact, "role") {
	case "ticker":
		for _, symbol := range krakenmarket.PayloadSymbols(artifact) {
			crypto.scopes.Store(symbol, struct{}{})
		}

		crypto.publishTickerMarks(artifact)

		replay.IngestTickerBatch(krakenmarket.MarketTree(), artifact)

		crypto.updateSignals(
			artifact,
			"causal",
			"correlation",
			"depthflow",
			"fluid",
			"leadlag",
			"liquidity",
			"manifold",
			"resonance",
		)
	case "book":
		for _, symbol := range krakenmarket.PayloadSymbols(artifact) {
			crypto.scopes.Store(symbol, struct{}{})
		}

		replay.IngestBookBatch(krakenmarket.MarketTree(), artifact)

		crypto.updateSignals(
			artifact,
			"causal",
			"depthflow",
			"fluid",
			"leadlag",
			"liquidity",
			"manifold",
			"resonance",
		)
	case "trade":
		for _, symbol := range krakenmarket.PayloadSymbols(artifact) {
			crypto.scopes.Store(symbol, struct{}{})
		}

		replay.IngestTradeBatch(krakenmarket.MarketTree(), artifact)

		crypto.updateSignals(
			artifact,
			"causal",
			"correlation",
			"depthflow",
			"fluid",
			"leadlag",
			"liquidity",
			"manifold",
			"resonance",
		)
	case "ohlc":
		updates := datura.As[krakenmarket.CandleUpdates](artifact)
		crypto.ohlc.Update(updates)

		for _, candle := range updates {
			errnie.Error(ui.PublishOhlc(crypto.pool, candle))
		}
	case "balances":
		payload := datura.As[user.Balances](artifact)
		crypto.balances.Update(payload)
		crypto.story.Update(artifact)
		errnie.Error(ui.PublishWallet(crypto.pool, &payload))
	case "status":
		if datura.Peek[string](artifact, "scope") == "connected" {
			crypto.resubscribePublic()
		}
	case "instrument":
		update := datura.As[krakenmarket.InstrumentUpdate](artifact)
		_, updateErr := crypto.instrument.Update(update)
		errnie.Error(updateErr)
		errnie.Error(crypto.instrument.SubscribeSymbols())
		crypto.applyInstrumentIncrements(update)
		replay.IngestInstrumentUpdate(update)
	case "execution":
		update := datura.As[user.Execution](artifact)
		crypto.execution.Update(update)
		crypto.recordExecutionOutcome(update)
	}

	return nil
}

func (crypto *Crypto) resubscribePublic() {
	errnie.Error(crypto.instrument.Subscribe())
	errnie.Error(crypto.instrument.SubscribeSymbols())
	errnie.Error(crypto.instrument.SubscribeAnchorOhlc())
}

func (crypto *Crypto) measure() {
	scopes := crypto.collectMeasureScopes()
	resonanceResults, settleErr := crypto.resonanceSignal.SettleScopes(scopes)

	errnie.Error(settleErr)

	for _, resMeasurement := range resonanceResults {
		if resMeasurement.Symbol == "" {
			continue
		}

		crypto.recordMeasurement(resMeasurement, nil)
	}

	dashboardSignals := crypto.dashboardSignalNames()

	for _, scope := range scopes {
		if scope == "" {
			continue
		}

		probe := datura.Acquire("trader", datura.Artifact_Type_json).
			WithRole("measurement").
			WithScope(scope)
		crypto.measureSignals(scope, probe, dashboardSignals)
	}

	eventAt := time.Now()

	if crypto.cognitiveMemory != nil {
		readings := crypto.cognitiveMemory.SealAllScopes(scopes, eventAt)
		crypto.cognitiveMemory.MaybeConsolidate(eventAt)

		for _, reading := range readings {
			if reading == nil {
				continue
			}

			crypto.execution.PreWarm(crypto.cognitiveMemory.PreWarmStaging(reading))
		}

		crypto.publishCognitiveReadings(readings)
	}

	errnie.Error(ui.PublishMeasurements(
		crypto.pool,
		crypto.story.Measurements(),
		crypto.storyTicks.Add(1),
		crypto.story.PlaybookEvaluationCount(),
		crypto.story.AnchorWalkTrace(),
	))
	errnie.Error(ui.PublishWallet(crypto.pool, crypto.balances.Snapshot()))
	crypto.publishFieldSnapshots()

	measurements := crypto.story.Measurements()

	errnie.Error(ui.PublishPayload(
		crypto.pool,
		"regime",
		logic.MarketRegimeFrame(measurements),
	))

	crypto.publishPredictionFrames()
}

func (crypto *Crypto) publishPredictionFrames() {
	if crypto.resonanceSignal == nil {
		return
	}

	for _, frame := range crypto.resonanceSignal.PredictionFrames(60) {
		errnie.Error(ui.PublishPayload(crypto.pool, "prediction", frame))
	}
}

func (crypto *Crypto) collectMeasureScopes() []string {
	scopes := make([]string, 0)

	crypto.scopes.Range(func(key, value any) bool {
		scope, ok := key.(string)

		if !ok || scope == "" {
			return true
		}

		scopes = append(scopes, scope)

		return true
	})

	return scopes
}

func (crypto *Crypto) publishFieldSnapshots() {
	at := time.Now()

	if crypto.fluidSignal != nil {
		payload, snapshotErr := crypto.fluidSignal.FieldSnapshot(at)

		errnie.Error(snapshotErr)
		errnie.Error(ui.PublishPayload(crypto.pool, "fluid", payload))
	}

	if crypto.manifoldSignal != nil {
		payload, snapshotErr := crypto.manifoldSignal.FieldSnapshot(at)

		errnie.Error(snapshotErr)
		errnie.Error(ui.PublishPayload(crypto.pool, "manifold", payload))
	}
}

func (crypto *Crypto) evaluateAttentionGating(
	symbol string,
	currentSurprise float64,
) bool {
	warmupSamples := viper.GetInt("signals.resonance.attention_warmup")

	if warmupSamples <= 0 {
		warmupSamples = 10
	}

	raw, _ := crypto.resonanceSurprise.LoadOrStore(
		symbol, &surpriseStats{n: 1, mean: currentSurprise},
	)
	stats := raw.(*surpriseStats)

	stats.n++
	delta := currentSurprise - stats.mean
	stats.mean += delta / stats.n
	delta2 := currentSurprise - stats.mean
	stats.m2 += delta * delta2

	if stats.n < float64(warmupSamples) {
		return true
	}

	variance := stats.m2 / (stats.n - 1)

	if variance <= 0 {
		return currentSurprise > stats.mean
	}

	threshold := stats.mean + (crypto.surpriseThreshold * math.Sqrt(variance))

	return currentSurprise > threshold
}

func (crypto *Crypto) measureSignals(
	scope string,
	probe *datura.Artifact,
	signalNames []string,
) {
	for _, signalName := range signalNames {
		crypto.measureSignal(scope, probe, signalName)
	}
}

func (crypto *Crypto) measureSignal(
	scope string,
	probe *datura.Artifact,
	signalName string,
) {
	if probe == nil {
		return
	}

	query := *probe

	var artifact *datura.Artifact

	switch signalName {
	case "causal":
		if crypto.causalSignal == nil {
			return
		}

		artifact = crypto.causalSignal.Measure(query)
	case "correlation":
		if crypto.correlationSignal == nil {
			return
		}

		artifact = crypto.correlationSignal.Measure(query)
	case "cvd":
		if crypto.cvdSignal == nil {
			return
		}

		artifact = crypto.cvdSignal.Measure(query)
	case "depthflow":
		if crypto.depthflowSignal == nil {
			return
		}

		artifact = crypto.depthflowSignal.Measure(query)
	case "exhaust":
		if crypto.exhaustSignal == nil {
			return
		}

		artifact = crypto.exhaustSignal.Measure(query)
	case "fluid":
		if crypto.fluidSignal == nil {
			return
		}

		artifact = crypto.fluidSignal.Measure(query)
	case "hawkes":
		if crypto.hawkesSignal == nil {
			return
		}

		artifact = crypto.hawkesSignal.Measure(query)
	case "leadlag":
		if crypto.leadlagSignal == nil {
			return
		}

		artifact = crypto.leadlagSignal.Measure(query)
	case "liquidity":
		if crypto.liquiditySignal == nil {
			return
		}

		artifact = crypto.liquiditySignal.Measure(query)
	case "manifold":
		if crypto.manifoldSignal == nil {
			return
		}

		artifact = crypto.manifoldSignal.Measure(query)
	case "pumpdump":
		if crypto.pumpdumpSignal == nil {
			return
		}

		artifact = crypto.pumpdumpSignal.Measure(query)
	case "sentiment":
		if crypto.sentimentSignal == nil {
			return
		}

		artifact = crypto.sentimentSignal.Measure(query)
	case "toxicity":
		if crypto.toxicitySignal == nil {
			return
		}

		artifact = crypto.toxicitySignal.Measure(query)
	default:
		return
	}

	if artifact == nil {
		return
	}

	crypto.enrichMeasurementArtifact(scope, artifact)

	errnie.Error(crypto.story.Update(artifact))
	crypto.observeSignalArtifact(scope, signalName, artifact)
}

func (crypto *Crypto) recordMeasurement(
	measurement logic.Measurement,
	measureErr error,
) {
	if measureErr != nil {
		errnie.Error(measureErr)

		return
	}

	if measurement.Source == "" {
		return
	}

	payload, err := json.Marshal(measurement)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"crypto: failed to marshal measurement",
			err,
		))

		return
	}

	artifact := datura.Acquire("trader", datura.Artifact_Type_json).
		WithRole("measurement").
		WithScope(measurement.Symbol).
		WithPayload(payload)

	errnie.Error(crypto.story.Update(artifact))

	if crypto.cognitiveMemory != nil {
		crypto.cognitiveMemory.ObserveMeasurement(measurement)
	}
}

func (crypto *Crypto) publishTickerMarks(artifact *datura.Artifact) {
	krakenmarket.VisitTickers(artifact, func(symbol string, last float64) bool {
		if !crypto.shouldPublishMark(symbol) {
			return true
		}

		errnie.Error(ui.PublishMark(crypto.pool, symbol, last))

		return true
	})
}

func (crypto *Crypto) shouldPublishMark(symbol string) bool {
	anchor := strings.TrimSpace(viper.GetString("market.anchor_symbol"))

	if anchor != "" && symbol == anchor {
		return true
	}

	snapshot := crypto.balances.Snapshot()

	if snapshot == nil || len(snapshot.Inventory) == 0 {
		return false
	}

	parts := strings.Split(symbol, "/")

	if len(parts) != 2 {
		return false
	}

	baseAsset := strings.ToUpper(parts[0])
	quoteAsset := strings.ToUpper(parts[1])
	quoteCurrency := strings.ToUpper(viper.GetString("market.quote_currency"))

	if quoteAsset != quoteCurrency {
		return false
	}

	quantity, held := snapshot.Inventory[baseAsset]

	return held && quantity > 0
}

func (crypto *Crypto) Close() error {
	crypto.cancel()

	if crypto.cognitiveMemory != nil {
		errnie.Error(crypto.cognitiveMemory.Close())
	}

	return nil
}

func (crypto *Crypto) enrichMeasurementArtifact(scope string, artifact *datura.Artifact) {
	if crypto.fluidSignal == nil || artifact == nil || scope == "" {
		return
	}

	facts := crypto.fluidSignal.MarketFacts(scope)

	if facts.Price > 0 {
		artifact.WithAttribute("price", facts.Price)
	}

	if facts.Volume >= 0 {
		artifact.WithAttribute("volume", facts.Volume)
	}

	if facts.Spread >= 0 {
		artifact.WithAttribute("spread", facts.Spread)
	}

	if facts.Elapsed >= 0 {
		artifact.WithAttribute("elapsed", facts.Elapsed)
	}

	if facts.Surprise >= 0 {
		artifact.WithAttribute("surprise", facts.Surprise)
	}

	if !facts.ObservedAt.IsZero() {
		artifact.WithAttribute("observed_at", facts.ObservedAt.UTC().Format(time.RFC3339Nano))
	}
}

func (crypto *Crypto) observeSignalArtifact(
	scope string,
	signalName string,
	artifact *datura.Artifact,
) {
	if crypto.cognitiveMemory == nil || artifact == nil || scope == "" {
		return
	}

	crypto.cognitiveMemory.ObserveArtifact(scope, signalName, artifact)
}

func (crypto *Crypto) applyCognitiveAction(action *logic.Action) bool {
	if action == nil {
		return false
	}

	if crypto.cognitiveMemory == nil {
		return true
	}

	if crypto.cognitiveMemory.Sideline(action.Symbol) {
		return false
	}

	reading, ok := crypto.cognitiveMemory.ReadingForScope(action.Symbol)

	if !ok || reading == nil {
		return true
	}

	crypto.cognitiveMemory.ApplyAction(action, reading)

	return true
}

func (crypto *Crypto) recordExecutionOutcome(update user.Execution) {
	if crypto.cognitiveMemory == nil || update.Symbol == "" {
		return
	}

	if update.ExecType != "trade" || update.LastQty <= 0 {
		return
	}

	reading, ok := crypto.cognitiveMemory.ReadingForScope(update.Symbol)

	if !ok || reading == nil {
		return
	}

	slippageBps := 0.0

	if update.LimitPrice > 0 && update.LastPrice > 0 {
		slippageBps = math.Abs(update.LastPrice-update.LimitPrice) / update.LimitPrice * 10000.0
	}

	sizeFraction := update.LastQty

	if update.OrderQty > 0 {
		sizeFraction = update.LastQty / update.OrderQty
	}

	profile := cognitive.ProfileFromExecution(slippageBps, sizeFraction)

	crypto.cognitiveMemory.RecordOutcome(
		reading.Sequence,
		profile,
		time.Now().UnixNano(),
	)
}

func (crypto *Crypto) publishCognitiveReadings(readings []*cognitive.Reading) {
	for _, reading := range readings {
		if reading == nil || reading.Scope == "" {
			continue
		}

		staging, hasStaging := crypto.execution.Staging(reading.Scope)

		frame := map[string]any{
			"type":              "cognitive",
			"scope":             reading.Scope,
			"sequence":          string(reading.Sequence),
			"regime_prefix":     string(reading.RegimePrefix),
			"regime_cohort":     reading.RegimeCohort,
			"ambiguous":         reading.Ambiguous,
			"sideline":          reading.Sideline,
			"entropy_bits":      reading.EntropyBits,
			"entropy_threshold": reading.EntropyThreshold,
			"class_confidence":  reading.ClassConfidence,
			"contrast_evidence": reading.ContrastEvidence,
			"lookahead_score":   reading.LookaheadScore,
			"lookahead_paths":   reading.LookaheadPaths,
			"winner_class":      string(reading.WinnerClass),
		}

		if hasStaging {
			frame["prewarm_paths"] = staging.LookaheadPaths
			frame["prewarm_score"] = staging.LookaheadScore
		}

		errnie.Error(ui.PublishPayload(crypto.pool, "cognitive", frame))
	}
}

func (crypto *Crypto) applyInstrumentIncrements(update krakenmarket.InstrumentUpdate) {
	if crypto.fluidSignal == nil {
		return
	}

	for _, pair := range update.Pairs {
		if pair.Symbol == "" || pair.PriceIncrement <= 0 {
			continue
		}

		crypto.fluidSignal.SetInstrumentTickSize(pair.Symbol, pair.PriceIncrement)
	}
}

func (crypto *Crypto) updateSignals(
	artifact *datura.Artifact,
	signals ...string,
) error {
	if artifact == nil || len(signals) == 0 {
		return nil
	}

	for _, name := range signals {
		if crypto.ctx.Err() != nil {
			return crypto.ctx.Err()
		}

		if updateErr := crypto.updateSignalByName(name, artifact); updateErr != nil {
			return errnie.Error(updateErr)
		}
	}

	return nil
}

func (crypto *Crypto) updateSignalByName(name string, artifact *datura.Artifact) error {
	switch name {
	case "causal":
		if crypto.causalSignal == nil {
			return nil
		}

		return crypto.causalSignal.Update(artifact)
	case "correlation":
		if crypto.correlationSignal == nil {
			return nil
		}

		return crypto.correlationSignal.Update(artifact)
	case "depthflow":
		if crypto.depthflowSignal == nil {
			return nil
		}

		return crypto.depthflowSignal.Update(artifact)
	case "fluid":
		if crypto.fluidSignal == nil {
			return nil
		}

		return crypto.fluidSignal.Update(artifact)
	case "leadlag":
		if crypto.leadlagSignal == nil {
			return nil
		}

		return crypto.leadlagSignal.Update(artifact)
	case "liquidity":
		if crypto.liquiditySignal == nil {
			return nil
		}

		return crypto.liquiditySignal.Update(artifact)
	case "manifold":
		if crypto.manifoldSignal == nil {
			return nil
		}

		return crypto.manifoldSignal.Update(artifact)
	case "resonance":
		if crypto.resonanceSignal == nil {
			return nil
		}

		return crypto.resonanceSignal.Update(artifact)
	default:
		return errnie.Err(
			errnie.Validation,
			fmt.Sprintf("crypto: unknown signal %q", name),
			nil,
		)
	}
}
