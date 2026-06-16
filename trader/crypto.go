package trader

import (
	"context"
	"encoding/json"
	"math"
	"sync"
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
	signals           *sync.Map
	scopes            *sync.Map
	story             *market.Story
	balancesSub       sync.Once
	instrumentSub     sync.Once
	resonanceSurprise *sync.Map
	surpriseThreshold float64
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
		signals:           &sync.Map{},
		scopes:            &sync.Map{},
		story:             market.NewStory(ctx, pool),
		resonanceSurprise: &sync.Map{},
		surpriseThreshold: viper.GetFloat64("signals.resonance.surprise_threshold"),
	}

	if crypto.surpriseThreshold <= 0 {
		crypto.surpriseThreshold = 1.5
	}

	for _, channel := range []string{
		"desk", "ui",
	} {
		crypto.broadcasts.Store(
			channel, pool.CreateBroadcastGroup(channel),
		)
	}

	for _, channel := range []string{
		"ticker", "book", "trade", "instrument", "action", "execution", "balances",
	} {
		crypto.subscribers.Store(
			channel, pool.Subscribe(channel, crypto.onMessage),
		)
	}

	return crypto
}

func (crypto *Crypto) Run() error {
	errnie.Error(crypto.balances.Subscribe())
	errnie.Error(crypto.instrument.Subscribe())

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
	switch artifact.Peek("role") {
	case "ticker":
		updates := datura.As[krakenmarket.TickerUpdates](artifact)
		crypto.ticker.Update(updates)

		for _, update := range updates {
			if update != nil && update.Symbol != "" {
				crypto.scopes.Store(update.Symbol, struct{}{})
			}
		}

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
	case "balances":
		payload := datura.As[user.Balances](artifact)
		crypto.balances.Update(payload)
		crypto.story.Update(artifact)
	case "instrument":
		update := datura.As[krakenmarket.InstrumentUpdate](artifact)
		errnie.Error(crypto.instrument.Update(update))
	}

	return nil
}

func (crypto *Crypto) requestBalancesSubscribe() {
	crypto.balancesSub.Do(func() {
		payload, err := json.Marshal(map[string]string{
			"method": "subscribe",
		})

		if err != nil {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"crypto: failed to marshal balances subscribe",
				err,
			))

			return
		}

		artifact := datura.Acquire("crypto", datura.Artifact_Type_json).
			WithDestination("kraken:private").
			WithRole("balances").
			WithPayload(payload)

		errnie.Error(crypto.pool.CreateBroadcastGroup("kraken:private").Send(artifact))
	})
}

func (crypto *Crypto) measure() {
	crypto.scopes.Range(func(key, value any) bool {
		scope, ok := key.(string)

		if !ok || scope == "" {
			return true
		}

		probe := datura.Acquire("trader", datura.Artifact_Type_json).
			WithRole("measurement").
			WithScope(scope)

		resMeasurement, measureErr := crypto.resonanceSignal().Measure(probe)

		if measureErr != nil {
			errnie.Error(measureErr)

			return true
		}

		crypto.recordMeasurement(resMeasurement, nil)

		if !crypto.evaluateAttentionGating(scope, resMeasurement.Surprise) {
			return true
		}

		crypto.measureScopeHeavy(probe)

		return true
	})

	errnie.Error(ui.PublishMeasurements(crypto.pool, crypto.story.Measurements()))
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

func (crypto *Crypto) resonanceSignal() *resonance.Signal {
	arch := viper.GetIntSlice("signals.resonance.arch")

	if len(arch) == 0 {
		arch = []int{4, 8, 3}
	}

	alpha := viper.GetFloat64("signals.resonance.alpha")

	if alpha <= 0 {
		alpha = 0.01
	}

	signal, _ := crypto.signals.LoadOrStore(
		"resonance", resonance.NewSignal(crypto.ctx, crypto.pool, arch, alpha),
	)

	return signal.(*resonance.Signal)
}

func (crypto *Crypto) measureScopeHeavy(probe *datura.Artifact) {
	signal, _ := crypto.signals.LoadOrStore(
		"causal", causal.NewSignal(crypto.ctx, crypto.pool),
	)
	crypto.recordMeasurement(signal.(*causal.Signal).Measure(probe))

	signal, _ = crypto.signals.LoadOrStore(
		"correlation", correlation.NewSignal(crypto.ctx, crypto.pool),
	)
	crypto.recordMeasurement(signal.(*correlation.Signal).Measure(probe))

	signal, _ = crypto.signals.LoadOrStore(
		"cvd", cvd.NewSignal(crypto.ctx, crypto.pool),
	)
	crypto.recordMeasurement(signal.(*cvd.Signal).Measure(probe))

	signal, _ = crypto.signals.LoadOrStore(
		"depthflow", depthflow.NewSignal(crypto.ctx, crypto.pool),
	)
	crypto.recordMeasurement(signal.(*depthflow.Signal).Measure(probe))

	signal, _ = crypto.signals.LoadOrStore(
		"exhaust", exhaust.NewSignal(crypto.ctx, crypto.pool),
	)
	crypto.recordMeasurement(signal.(*exhaust.Signal).Measure(probe))

	signal, _ = crypto.signals.LoadOrStore(
		"fluid", fluid.NewSignal(crypto.ctx, crypto.pool),
	)
	crypto.recordMeasurement(signal.(*fluid.Signal).Measure(probe))

	signal, _ = crypto.signals.LoadOrStore(
		"hawkes", hawkes.NewSignal(crypto.ctx, crypto.pool),
	)
	crypto.recordMeasurement(signal.(*hawkes.Signal).Measure(probe))

	signal, _ = crypto.signals.LoadOrStore(
		"leadlag", leadlag.NewSignal(crypto.ctx, crypto.pool),
	)
	crypto.recordMeasurement(signal.(*leadlag.Signal).Measure(probe))

	signal, _ = crypto.signals.LoadOrStore(
		"liquidity", liquidity.NewSignal(crypto.ctx, crypto.pool),
	)
	crypto.recordMeasurement(signal.(*liquidity.Signal).Measure(probe))

	signal, _ = crypto.signals.LoadOrStore(
		"manifold", manifold.NewSignal(crypto.ctx, crypto.pool),
	)
	crypto.recordMeasurement(signal.(*manifold.Signal).Measure(probe))

	signal, _ = crypto.signals.LoadOrStore(
		"prediction", prediction.NewSignal(crypto.ctx, crypto.pool),
	)
	crypto.recordMeasurement(signal.(*prediction.Signal).Measure(probe))

	signal, _ = crypto.signals.LoadOrStore(
		"pumpdump", pumpdump.NewSignal(crypto.ctx, crypto.pool),
	)
	crypto.recordMeasurement(signal.(*pumpdump.Signal).Measure(probe))

	signal, _ = crypto.signals.LoadOrStore(
		"sentiment", sentiment.NewSignal(crypto.ctx, crypto.pool),
	)
	crypto.recordMeasurement(signal.(*sentiment.Signal).Measure(probe))

	signal, _ = crypto.signals.LoadOrStore(
		"toxicity", toxicity.NewSignal(crypto.ctx, crypto.pool),
	)
	crypto.recordMeasurement(signal.(*toxicity.Signal).Measure(probe))
}

func (crypto *Crypto) recordMeasurement(
	measurement logic.Measurement,
	measureErr error,
) {
	if measureErr != nil {
		errnie.Error(measureErr)

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

func (crypto *Crypto) Close() error {
	crypto.cancel()
	return nil
}

func (crypto *Crypto) updateSignals(
	artifact *datura.Artifact,
	signals ...string,
) error {
	for _, name := range signals {
		crypto.pool.ScheduleFast(func() {
			switch name {
			case "causal":
				signal, _ := crypto.signals.LoadOrStore(
					name, causal.NewSignal(crypto.ctx, crypto.pool),
				)

				errnie.Error(signal.(*causal.Signal).Update(artifact))
			case "correlation":
				signal, _ := crypto.signals.LoadOrStore(
					name, correlation.NewSignal(crypto.ctx, crypto.pool),
				)

				errnie.Error(signal.(*correlation.Signal).Update(artifact))
			case "cvd":
				signal, _ := crypto.signals.LoadOrStore(
					name, cvd.NewSignal(crypto.ctx, crypto.pool),
				)

				errnie.Error(signal.(*cvd.Signal).Update(artifact))
			case "depthflow":
				signal, _ := crypto.signals.LoadOrStore(
					name, depthflow.NewSignal(crypto.ctx, crypto.pool),
				)

				errnie.Error(signal.(*depthflow.Signal).Update(artifact))
			case "exhaust":
				signal, _ := crypto.signals.LoadOrStore(
					name, exhaust.NewSignal(crypto.ctx, crypto.pool),
				)

				errnie.Error(signal.(*exhaust.Signal).Update(artifact))
			case "fluid":
				signal, _ := crypto.signals.LoadOrStore(
					name, fluid.NewSignal(crypto.ctx, crypto.pool),
				)

				errnie.Error(signal.(*fluid.Signal).Update(artifact))
			case "hawkes":
				signal, _ := crypto.signals.LoadOrStore(
					name, hawkes.NewSignal(crypto.ctx, crypto.pool),
				)

				errnie.Error(signal.(*hawkes.Signal).Update(artifact))
			case "leadlag":
				signal, _ := crypto.signals.LoadOrStore(
					name, leadlag.NewSignal(crypto.ctx, crypto.pool),
				)

				errnie.Error(signal.(*leadlag.Signal).Update(artifact))
			case "liquidity":
				signal, _ := crypto.signals.LoadOrStore(
					name, liquidity.NewSignal(crypto.ctx, crypto.pool),
				)

				errnie.Error(signal.(*liquidity.Signal).Update(artifact))
			case "manifold":
				signal, _ := crypto.signals.LoadOrStore(
					name, manifold.NewSignal(crypto.ctx, crypto.pool),
				)

				errnie.Error(signal.(*manifold.Signal).Update(artifact))
			case "prediction":
				signal, _ := crypto.signals.LoadOrStore(
					name, prediction.NewSignal(crypto.ctx, crypto.pool),
				)

				errnie.Error(signal.(*prediction.Signal).Update(artifact))
			case "pumpdump":
				signal, _ := crypto.signals.LoadOrStore(
					name, pumpdump.NewSignal(crypto.ctx, crypto.pool),
				)

				errnie.Error(signal.(*pumpdump.Signal).Update(artifact))
			case "resonance":
				errnie.Error(crypto.resonanceSignal().Update(artifact))
			case "sentiment":
				signal, _ := crypto.signals.LoadOrStore(
					name, sentiment.NewSignal(crypto.ctx, crypto.pool),
				)

				errnie.Error(signal.(*sentiment.Signal).Update(artifact))
			case "toxicity":
				signal, _ := crypto.signals.LoadOrStore(
					name, toxicity.NewSignal(crypto.ctx, crypto.pool),
				)

				errnie.Error(signal.(*toxicity.Signal).Update(artifact))
			}
		})
	}

	return nil
}
