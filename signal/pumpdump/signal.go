package pumpdump

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/market/perspectives/types"
	"github.com/theapemachine/symm/numeric"
	"github.com/theapemachine/symm/numeric/adaptive"
	"github.com/theapemachine/symm/rawdump"
	signalpool "github.com/theapemachine/symm/signal"
)

const rawSubscriberID = "signal/pumpdump:raw"

/*
Signal measuring Pump and Dump market dynamics — the ignition perspective.

It reads the trade tape and looks for sudden verticality: a volume spike (RVOL)
detaching from the symbol's own recent norm, optionally amplified by a precursor
price move off the window's opening anchor. Both axes are self-scaling — read as
value / EMA(value), so "high", "moderate" and "falling" mean relative to this
symbol's own recent behaviour, never a hard-coded level — then fused, smoothed,
sigma-clamped, and banded into the four ignition categories:

	| Category           | Volume Lift | Price Precursor | Market "Feel"        |
	|:-------------------|:------------|:----------------|:---------------------|
	| Vertical Ignition  | High Spike  | High            | Launching / Breakout |
	| Coiled Compression | Moderate    | Low             | Pre-Pump / Loaded    |
	| Organic Trend      | Low/Steady  | Moderate        | Healthy Momentum     |
	| Faded Exhaustion   | Falling     | Flat            | Leg is Dead          |

Mechanics: per symbol it maintains gross and signed volume over a configured
horizon, self-scaled RVOL and signed price precursor off the window anchor, and
tape skew. Ignition lift is (rvol-1)*(1+precursor)*(1+skew); a pooled
BandCalibrator bands that scalar into the four categories by live quantile
position. Strength is the banded lift observation. Spread compression (a third
axis in the written design) needs the book and is left to the book-driven signals.
*/
type Signal struct {
	ctx           context.Context
	cancel        context.CancelFunc
	pool          *qpool.Q[any]
	window        time.Duration
	broadcasts    map[string]*qpool.BroadcastGroup
	subscribers   map[string]*qpool.BroadcastConsumer
	symbols         sync.Map
	tickerTracks    sync.Map
	lastMeasurement types.Measurement
	categories      map[string]types.CategoryType
	surpriseField *types.CategorySurpriseField
	rawDump       *rawdump.Writer
	classifier    *adaptive.Classifier
	calibrator    *numeric.BandCalibrator
}

var pumpDefaultBandEdges = []float64{-0.10, 0.50, 2.00}

func NewSignal(ctx context.Context, pool *qpool.Q[any]) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	window := viper.GetDuration("signals.pumpdump.window")

	if window <= 0 {
		errnie.Error(errors.New("signals.pumpdump.window is required"))
	}

	calibrator := numeric.NewSignalCalibrator(
		pumpDefaultBandEdges,
		[]float64{0, 1, 2, 3},
		[]string{"faded_exhaustion", "organic_trend", "coiled_compression", "vertical_ignition"},
		[]float64{0.50, 0.30, 0.15, 0.05},
		numeric.DefaultCalibratorConfig("observation"),
		"pumpdump",
	)

	surpriseField, err := types.NewCategorySurpriseField([]types.CategoryType{
		types.CategoryFadedExhaustion,
		types.CategoryOrganicTrend,
		types.CategoryCoiledCompression,
		types.CategoryVerticalIgnition,
	}, types.DefaultCategorySurpriseAlpha)

	if err != nil {
		cancel()
		errnie.Error(err, "signal/pumpdump")
		return nil
	}

	signal := &Signal{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		window:      window,
		broadcasts:  make(map[string]*qpool.BroadcastGroup),
		subscribers: make(map[string]*qpool.BroadcastConsumer),
		categories: map[string]types.CategoryType{
			"faded_exhaustion":   types.CategoryFadedExhaustion,
			"organic_trend":      types.CategoryOrganicTrend,
			"coiled_compression": types.CategoryCoiledCompression,
			"vertical_ignition":  types.CategoryVerticalIgnition,
		},
		surpriseField: surpriseField,
		rawDump:       rawdump.Open("pumpdump"),
		classifier:    calibrator.Classifier,
		calibrator:    calibrator.Calibrator,
	}

	queueTTL := viper.GetDuration("system.queue.ttl")

	for _, channel := range []string{"raw"} {
		signal.broadcasts[channel] = pool.CreateBroadcastGroup(channel, queueTTL)
		signal.subscribers[channel] = signal.broadcasts[channel].Subscribe(rawSubscriberID, 1024)
	}

	signal.broadcasts["measurements"] = pool.CreateBroadcastGroup("measurements", queueTTL)
	signal.broadcasts["ui"] = pool.CreateBroadcastGroup("ui", queueTTL)

	errnie.Info("signal/pumpdump ready", "signal/pumpdump")

	return signal
}

func (signal *Signal) stateFor(symbol string) *pumpState {
	stored, _ := signal.symbols.LoadOrStore(
		symbol, newPumpState(signal.classifier, signal.window),
	)

	return stored.(*pumpState)
}

func (signal *Signal) Tick() error {
	for {
		message, err := signal.subscribers["raw"].Wait(signal.ctx)
		if err != nil {
			return err
		}

		if message == nil {
			continue
		}

		errnie.Debug("signal/pumpdump: Tick()", "type", message.Type)

		sm, ok := signalpool.SocketMessageFromValue(message.Value)

		if !ok {
			continue
		}

		switch sm.Channel {
		case public.TradesChannel:
			for _, trade := range signalpool.GetTrades(sm) {
				err := signal.observe(trade)

				if err != nil && !isWarmup(err) {
					errnie.Error(err, "pumpdump: observe %s", trade.Symbol)
				}
			}
		case public.TickerChannel:
			for _, ticker := range signalpool.GetTickers(sm) {
				err := signal.observeTicker(ticker)

				if err != nil && !isWarmup(err) {
					errnie.Error(err, "pumpdump: observe ticker %s", ticker.Symbol)
				}
			}
		}
	}
}

func (signal *Signal) observe(trade market.TradeUpdate) error {
	if trade.Price <= 0 || trade.Qty <= 0 {
		return errnie.Error(errors.New("pumpdump: price and quantity are required"))
	}

	state := signal.stateFor(trade.Symbol)
	reading, err := state.fold(trade)

	if err != nil {
		if isWarmup(err) {
			return err
		}

		return errnie.Error(err, "pumpdump: fold %s", trade.Symbol)
	}

	return signal.publish(trade, reading)
}

func (signal *Signal) tickerTrackFor(symbol string) *tickerTrack {
	stored, _ := signal.tickerTracks.LoadOrStore(symbol, &tickerTrack{})

	return stored.(*tickerTrack)
}

func (signal *Signal) observeTicker(ticker market.TickerUpdate) error {
	if ticker.Last <= 0 {
		return nil
	}

	at, err := tickerTimestamp(ticker)

	if err != nil {
		return errnie.Error(err, "pumpdump: ticker timestamp %s", ticker.Symbol)
	}

	state := signal.stateFor(ticker.Symbol)
	track := signal.tickerTrackFor(ticker.Symbol)
	reading, err := track.fold(state, ticker, at)

	if err != nil {
		if isWarmup(err) {
			return err
		}

		return errnie.Error(err, "pumpdump: fold ticker %s", ticker.Symbol)
	}

	trade := market.TradeUpdate{
		Symbol:    ticker.Symbol,
		Price:     ticker.Last,
		Timestamp: at,
	}

	return signal.publish(trade, reading)
}

func (signal *Signal) Close() error {
	signal.cancel()

	return signal.rawDump.Close()
}
