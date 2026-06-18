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
		crypto.storePayloadScopes(artifact)
		crypto.publishTickerMarks(artifact)
	case "book":
		crypto.storePayloadScopes(artifact)
	case "trade":
		crypto.storePayloadScopes(artifact)
	case "ohlc":
		crypto.publishOhlcFromArtifact(artifact)
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
		added, ingestErr := crypto.instrument.ingestCatalog(artifact)
		errnie.Error(ingestErr)

		if len(added) > 0 {
			errnie.Error(crypto.instrument.subscribeSymbolBatch(added))
		}

		crypto.applyInstrumentIncrements(artifact)
	case "execution":
		update := datura.As[user.Execution](artifact)
		crypto.execution.Update(update)
		crypto.recordExecutionOutcome(update)
	}

	return nil
}

func (crypto *Crypto) resubscribePublic() {
	errnie.Error(crypto.instrument.Subscribe())
	errnie.Error(crypto.instrument.subscribeKnownSymbols())
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
	publishRow := func(symbolPath, lastPath string) bool {
		symbol := datura.PeekPayload[string](artifact, symbolPath)
		last := datura.PeekPayload[float64](artifact, lastPath)

		if symbol == "" || last <= 0 {
			return true
		}

		if !crypto.shouldPublishMark(symbol) {
			return true
		}

		errnie.Error(ui.PublishMark(crypto.pool, symbol, last))

		return true
	}

	if count, ok := datura.PayloadLen(artifact); ok {
		for index := 0; index < count; index++ {
			prefix := fmt.Sprintf("%d", index)

			if !publishRow(prefix+".symbol", prefix+".last") {
				return
			}
		}

		return
	}

	for index := 0; ; index++ {
		prefix := fmt.Sprintf("data.%d", index)
		_, ok := datura.PeekPayloadOK[string](artifact, prefix+".symbol")

		if !ok {
			break
		}

		if !publishRow(prefix+".symbol", prefix+".last") {
			return
		}
	}
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

func (crypto *Crypto) applyInstrumentIncrements(artifact *datura.Artifact) {
	if crypto.fluidSignal == nil || artifact == nil {
		return
	}

	for index := 0; ; index++ {
		symbolPath := fmt.Sprintf("data.pairs.%d.symbol", index)
		symbol, ok := datura.PeekPayloadOK[string](artifact, symbolPath)

		if !ok || symbol == "" {
			break
		}

		increment := datura.PeekPayload[float64](
			artifact,
			fmt.Sprintf("data.pairs.%d.price_increment", index),
		)

		if increment <= 0 {
			continue
		}

		crypto.fluidSignal.SetInstrumentTickSize(symbol, increment)
	}
}

func (crypto *Crypto) storePayloadScopes(artifact *datura.Artifact) {
	if artifact == nil {
		return
	}

	seen := map[string]struct{}{}

	if scope, scopeErr := artifact.Scope(); scopeErr == nil && scope != "" {
		seen[scope] = struct{}{}
		crypto.scopes.Store(scope, struct{}{})
	}

	if count, ok := datura.PayloadLen(artifact); ok {
		for index := 0; index < count; index++ {
			crypto.storePayloadSymbol(
				artifact,
				fmt.Sprintf("%d.symbol", index),
				seen,
			)
		}

		return
	}

	for index := 0; ; index++ {
		path := fmt.Sprintf("data.%d.symbol", index)
		symbol, ok := datura.PeekPayloadOK[string](artifact, path)

		if !ok || symbol == "" {
			break
		}

		if _, exists := seen[symbol]; exists {
			continue
		}

		seen[symbol] = struct{}{}
		crypto.scopes.Store(symbol, struct{}{})
	}
}

func (crypto *Crypto) storePayloadSymbol(
	artifact *datura.Artifact,
	path string,
	seen map[string]struct{},
) {
	symbol, ok := datura.PeekPayloadOK[string](artifact, path)

	if !ok || symbol == "" {
		return
	}

	if _, exists := seen[symbol]; exists {
		return
	}

	seen[symbol] = struct{}{}
	crypto.scopes.Store(symbol, struct{}{})
}

func (crypto *Crypto) publishOhlcFromArtifact(artifact *datura.Artifact) {
	if artifact == nil {
		return
	}

	publishRow := func(prefix string) bool {
		symbol := datura.PeekPayload[string](artifact, prefix+".symbol")

		if symbol == "" {
			return true
		}

		open := datura.PeekPayload[float64](artifact, prefix+".open")
		high := datura.PeekPayload[float64](artifact, prefix+".high")
		low := datura.PeekPayload[float64](artifact, prefix+".low")
		closePrice := datura.PeekPayload[float64](artifact, prefix+".close")

		if !logic.ScalarFinite(open) ||
			!logic.ScalarFinite(high) ||
			!logic.ScalarFinite(low) ||
			!logic.ScalarFinite(closePrice) {
			return true
		}

		sec, secErr := ohlcUnixSec(
			datura.PeekPayload[string](artifact, prefix+".interval_begin"),
		)

		if secErr != nil {
			errnie.Error(secErr)

			return true
		}

		volume := datura.PeekPayload[float64](artifact, prefix+".volume")

		if !logic.ScalarFinite(volume) {
			volume = 0
		}

		errnie.Error(ui.PublishPayload(crypto.pool, "ohlc", map[string]any{
			"type":   "ohlc",
			"symbol": symbol,
			"sec":    sec,
			"open":   open,
			"high":   high,
			"low":    low,
			"close":  closePrice,
			"volume": volume,
		}))

		return true
	}

	if count, ok := datura.PayloadLen(artifact); ok {
		for index := 0; index < count; index++ {
			if !publishRow(fmt.Sprintf("%d", index)) {
				return
			}
		}

		return
	}

	for index := 0; ; index++ {
		prefix := fmt.Sprintf("data.%d", index)
		_, ok := datura.PeekPayloadOK[string](artifact, prefix+".symbol")

		if !ok {
			break
		}

		if !publishRow(prefix) {
			return
		}
	}
}

func ohlcUnixSec(intervalBegin string) (int64, error) {
	trimmed := strings.TrimSpace(intervalBegin)

	if trimmed == "" {
		return 0, errnie.Err(
			errnie.Validation,
			"crypto: ohlc interval_begin is empty",
			nil,
		)
	}

	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.000000Z",
	}

	for _, layout := range layouts {
		parsed, parseErr := time.Parse(layout, trimmed)

		if parseErr == nil {
			return parsed.Unix(), nil
		}
	}

	return 0, errnie.Err(
		errnie.Validation,
		"crypto: ohlc interval_begin is not parseable",
		nil,
	)
}

func (instrument *Instrument) ingestCatalog(artifact *datura.Artifact) ([]string, error) {
	if artifact == nil {
		return nil, nil
	}

	added := make([]string, 0)

	for index := 0; ; index++ {
		symbolPath := fmt.Sprintf("data.pairs.%d.symbol", index)
		symbol, ok := datura.PeekPayloadOK[string](artifact, symbolPath)

		if !ok || symbol == "" {
			break
		}

		quote := datura.PeekPayload[string](
			artifact,
			fmt.Sprintf("data.pairs.%d.quote", index),
		)
		status := datura.PeekPayload[string](
			artifact,
			fmt.Sprintf("data.pairs.%d.status", index),
		)

		if quote != instrument.quote || status != "online" {
			continue
		}

		if _, exists := instrument.known.LoadOrStore(symbol, struct{}{}); exists {
			continue
		}

		added = append(added, symbol)
	}

	return added, nil
}

func (instrument *Instrument) subscribeKnownSymbols() error {
	symbols := make([]string, 0)

	instrument.known.Range(func(key, value any) bool {
		symbol, ok := key.(string)

		if !ok || symbol == "" {
			return true
		}

		symbols = append(symbols, symbol)

		return true
	})

	return instrument.subscribeSymbolBatch(symbols)
}

func (instrument *Instrument) subscribeSymbolBatch(symbols []string) error {
	if len(symbols) == 0 {
		return nil
	}

	batchSize := viper.GetInt("market.subscribe_batch")

	if batchSize <= 0 {
		batchSize = 100
	}

	pace := viper.GetDuration("market.subscribe_pace")

	bookDepth := viper.GetInt("market.book.depth")

	if bookDepth <= 0 {
		bookDepth = viper.GetInt("market.book_depth_levels")
	}

	if bookDepth <= 0 {
		bookDepth = 10
	}

	for batchStart := 0; batchStart < len(symbols); batchStart += batchSize {
		batch := symbols[batchStart:min(batchStart+batchSize, len(symbols))]

		for _, trigger := range []string{"bbo", "trades"} {
			if err := instrument.sendSubscribe(map[string]any{
				"channel":        "ticker",
				"symbol":         batch,
				"snapshot":       true,
				"event_trigger":  trigger,
			}); err != nil {
				return err
			}

			if pace > 0 {
				time.Sleep(pace)
			}
		}

		if err := instrument.sendSubscribe(map[string]any{
			"channel":  "book",
			"symbol":   batch,
			"depth":    bookDepth,
			"snapshot": true,
		}); err != nil {
			return err
		}

		if pace > 0 {
			time.Sleep(pace)
		}

		if err := instrument.sendSubscribe(map[string]any{
			"channel":  "trade",
			"symbol":   batch,
			"snapshot": true,
		}); err != nil {
			return err
		}

		if pace > 0 {
			time.Sleep(pace)
		}
	}

	return nil
}
