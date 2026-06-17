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
	"github.com/theapemachine/symm/signal/prediction"
	"github.com/theapemachine/symm/signal/pumpdump"
	"github.com/theapemachine/symm/signal/resonance"
	"github.com/theapemachine/symm/signal/sentiment"
	"github.com/theapemachine/symm/signal/toxicity"
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
	predictionSignal  *prediction.Signal
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

				crypto.action.Update(*action)
			}
		}
	}
}

func (crypto *Crypto) onMessage(artifact *datura.Artifact) error {
	switch datura.Peek[string](artifact, "role") {
	case "ticker":
		updates := datura.As[krakenmarket.TickerUpdates](artifact)
		crypto.ticker.Update(updates)

		for _, update := range updates {
			if update != nil && update.Symbol != "" {
				crypto.scopes.Store(update.Symbol, struct{}{})
			}
		}

		crypto.publishTickerMarks(updates)

		crypto.updateSignals(
			artifact,
			"causal",
			"correlation",
			"depthflow",
			"exhaust",
			"fluid",
			"leadlag",
			"liquidity",
			"manifold",
			"resonance",
			"sentiment",
			"toxicity",
		)
	case "book":
		updates := datura.As[krakenmarket.BookUpdates](artifact)
		crypto.book.Update(updates)

		for _, update := range updates {
			if update != nil && update.Symbol != "" {
				crypto.scopes.Store(update.Symbol, struct{}{})
			}
		}

		crypto.updateSignals(
			artifact,
			"causal",
			"depthflow",
			"exhaust",
			"fluid",
			"leadlag",
			"liquidity",
			"manifold",
			"pumpdump",
			"resonance",
			"sentiment",
			"toxicity",
		)
	case "trade":
		updates := datura.As[krakenmarket.TradeUpdates](artifact)
		crypto.trade.Update(updates)

		for _, update := range updates {
			if update != nil && update.Symbol != "" {
				crypto.scopes.Store(update.Symbol, struct{}{})
			}
		}

		crypto.updateSignals(
			artifact,
			"causal",
			"correlation",
			"cvd",
			"depthflow",
			"exhaust",
			"fluid",
			"hawkes",
			"leadlag",
			"liquidity",
			"manifold",
			"prediction",
			"pumpdump",
			"resonance",
			"sentiment",
			"toxicity",
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

	for scope, resMeasurement := range resonanceResults {
		if resMeasurement.Symbol == "" {
			continue
		}

		crypto.recordMeasurement(resMeasurement, nil)

		if !crypto.evaluateAttentionGating(scope, resMeasurement.Surprise) {
			continue
		}

		targets := resonance.MeasureTargets(resMeasurement.Category)

		if len(targets) == 0 {
			continue
		}

		probe := datura.Acquire("trader", datura.Artifact_Type_json).
			WithRole("measurement").
			WithScope(scope)
		crypto.measureSignals(probe, targets)
	}

	errnie.Error(ui.PublishMeasurements(
		crypto.pool,
		crypto.story.Measurements(),
		crypto.storyTicks.Add(1),
	))
	errnie.Error(ui.PublishWallet(crypto.pool, crypto.balances.Snapshot()))
	crypto.publishFieldSnapshots()
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

func (crypto *Crypto) measureSignals(probe *datura.Artifact, signalNames []string) {
	for _, signalName := range signalNames {
		crypto.measureSignal(probe, signalName)
	}
}

func (crypto *Crypto) measureSignal(probe *datura.Artifact, signalName string) {
	switch signalName {
	case "causal":
		if crypto.causalSignal == nil {
			return
		}

		crypto.recordMeasurement(crypto.causalSignal.Measure(probe))
	case "correlation":
		if crypto.correlationSignal == nil {
			return
		}

		crypto.recordMeasurement(crypto.correlationSignal.Measure(probe))
	case "cvd":
		if crypto.cvdSignal == nil {
			return
		}

		crypto.recordMeasurement(crypto.cvdSignal.Measure(probe))
	case "depthflow":
		if crypto.depthflowSignal == nil {
			return
		}

		crypto.recordMeasurement(crypto.depthflowSignal.Measure(probe))
	case "exhaust":
		if crypto.exhaustSignal == nil {
			return
		}

		crypto.recordMeasurement(crypto.exhaustSignal.Measure(probe))
	case "fluid":
		if crypto.fluidSignal == nil {
			return
		}

		crypto.recordMeasurement(crypto.fluidSignal.Measure(probe))
	case "hawkes":
		if crypto.hawkesSignal == nil {
			return
		}

		crypto.recordMeasurement(crypto.hawkesSignal.Measure(probe))
	case "leadlag":
		if crypto.leadlagSignal == nil {
			return
		}

		crypto.recordMeasurement(crypto.leadlagSignal.Measure(probe))
	case "liquidity":
		if crypto.liquiditySignal == nil {
			return
		}

		crypto.recordMeasurement(crypto.liquiditySignal.Measure(probe))
	case "manifold":
		if crypto.manifoldSignal == nil {
			return
		}

		crypto.recordMeasurement(crypto.manifoldSignal.Measure(probe))
	case "prediction":
		if crypto.predictionSignal == nil {
			return
		}

		crypto.recordMeasurement(crypto.predictionSignal.Measure(probe))
	case "pumpdump":
		if crypto.pumpdumpSignal == nil {
			return
		}

		crypto.recordMeasurement(crypto.pumpdumpSignal.Measure(probe))
	case "sentiment":
		if crypto.sentimentSignal == nil {
			return
		}

		crypto.recordMeasurement(crypto.sentimentSignal.Measure(probe))
	case "toxicity":
		if crypto.toxicitySignal == nil {
			return
		}

		crypto.recordMeasurement(crypto.toxicitySignal.Measure(probe))
	}
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
}

func (crypto *Crypto) publishTickerMarks(updates krakenmarket.TickerUpdates) {
	for _, update := range updates {
		if update == nil || update.Symbol == "" || update.Last <= 0 {
			continue
		}

		if !crypto.shouldPublishMark(update.Symbol) {
			continue
		}

		errnie.Error(ui.PublishMark(crypto.pool, update.Symbol, update.Last))
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
	return nil
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
	case "cvd":
		if crypto.cvdSignal == nil {
			return nil
		}

		return crypto.cvdSignal.Update(artifact)
	case "depthflow":
		if crypto.depthflowSignal == nil {
			return nil
		}

		return crypto.depthflowSignal.Update(artifact)
	case "exhaust":
		if crypto.exhaustSignal == nil {
			return nil
		}

		return crypto.exhaustSignal.Update(artifact)
	case "fluid":
		if crypto.fluidSignal == nil {
			return nil
		}

		return crypto.fluidSignal.Update(artifact)
	case "hawkes":
		if crypto.hawkesSignal == nil {
			return nil
		}

		return crypto.hawkesSignal.Update(artifact)
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
	case "prediction":
		if crypto.predictionSignal == nil {
			return nil
		}

		return crypto.predictionSignal.Update(artifact)
	case "pumpdump":
		if crypto.pumpdumpSignal == nil {
			return nil
		}

		return crypto.pumpdumpSignal.Update(artifact)
	case "resonance":
		if crypto.resonanceSignal == nil {
			return nil
		}

		return crypto.resonanceSignal.Update(artifact)
	case "sentiment":
		if crypto.sentimentSignal == nil {
			return nil
		}

		return crypto.sentimentSignal.Update(artifact)
	case "toxicity":
		if crypto.toxicitySignal == nil {
			return nil
		}

		return crypto.toxicitySignal.Update(artifact)
	default:
		return errnie.Err(
			errnie.Validation,
			fmt.Sprintf("crypto: unknown signal %q", name),
			nil,
		)
	}
}
