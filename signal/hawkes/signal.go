package hawkes

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

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

var hawkesDefaultBandEdges = []float64{0.5, 1.0, 1.5}

const (
	tickCapacity    = 4096
	rawSubscriberID = "signal/hawkes:raw"
)

/*
Signal detects trade-cluster excitation via a bivariate self-exciting Hawkes
model and maps the fitted state onto the thermal perspective (Frenzy /
Saturation / Organic / Exhaustion). It consumes the executed trade tape; the
per-symbol fit is cooldown-throttled inside HawkesSymbol.
*/
type Signal struct {
	ctx           context.Context
	cancel        context.CancelFunc
	pool          *qpool.Q[any]
	broadcasts    map[string]*qpool.BroadcastGroup
	subscribers   map[string]*qpool.BroadcastConsumer
	symbols       sync.Map
	tradeScratch  []tradeTouch
	surpriseField *types.CategorySurpriseField
	classifier    *adaptive.Classifier
	calibrator    *numeric.BandCalibrator
	categories    map[string]types.CategoryType
	rawDump       *rawdump.Writer
}

type tradeTouch struct {
	symbol string
	state  *symbolState
	last   float64
}

/*
symbolState pairs one symbol's rolling trade window with its Hawkes fitter.
*/
type symbolState struct {
	mu     sync.Mutex
	hawkes *HawkesSymbol
	ticks  []market.TradeUpdate
}

func newSymbolState(categories map[string]types.CategoryType, classifier *adaptive.Classifier) *symbolState {
	return &symbolState{hawkes: NewHawkesSymbol(classifier, categories)}
}

func (state *symbolState) append(trade market.TradeUpdate) {
	state.mu.Lock()
	defer state.mu.Unlock()

	if len(state.ticks) >= tickCapacity {
		state.ticks = append(state.ticks[len(state.ticks)-tickCapacity+1:], trade)
		return
	}

	state.ticks = append(state.ticks, trade)
}

func (state *symbolState) measure(now time.Time) (types.Measurement, float64, error) {
	state.mu.Lock()
	defer state.mu.Unlock()

	return state.hawkes.Measure(state.ticks, now)
}

func NewSignal(ctx context.Context, pool *qpool.Q[any]) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	pooledCalibrator := numeric.NewSignalCalibrator(
		hawkesDefaultBandEdges,
		[]float64{0, 1, 2, 3},
		[]string{"organic", "frenzy", "saturation", "exhaustion"},
		[]float64{0.40, 0.30, 0.20, 0.10},
		numeric.DefaultCalibratorConfig("strength"),
		"hawkes",
	)

	categories := map[string]types.CategoryType{
		"organic":    types.CategoryOrganic,
		"frenzy":     types.CategoryFrenzy,
		"saturation": types.CategorySaturation,
		"exhaustion": types.CategoryExhaustion,
	}

	surpriseField, err := types.NewCategorySurpriseField([]types.CategoryType{
		types.CategoryOrganic,
		types.CategoryFrenzy,
		types.CategorySaturation,
		types.CategoryExhaustion,
	}, types.DefaultCategorySurpriseAlpha)

	if err != nil {
		cancel()
		errnie.Error(err, "signal/hawkes")
		return nil
	}

	signal := &Signal{
		ctx:           ctx,
		cancel:        cancel,
		pool:          pool,
		broadcasts:    make(map[string]*qpool.BroadcastGroup),
		subscribers:   make(map[string]*qpool.BroadcastConsumer),
		surpriseField: surpriseField,
		classifier:    pooledCalibrator.Classifier,
		calibrator:    pooledCalibrator.Calibrator,
		categories:    categories,
		rawDump:       rawdump.Open("hawkes"),
	}

	for _, channel := range []string{"raw"} {
		signal.broadcasts[channel] = pool.CreateBroadcastGroup(channel, 10*time.Millisecond)
		signal.subscribers[channel] = signal.broadcasts[channel].Subscribe(rawSubscriberID, 1024)
	}

	signal.broadcasts["measurements"] = pool.CreateBroadcastGroup("measurements", 10*time.Millisecond)
	signal.broadcasts["ui"] = pool.CreateBroadcastGroup("ui", 10*time.Millisecond)

	errnie.Info("signal/hawkes ready", "signal/hawkes")

	return signal
}

func (signal *Signal) Tick() error {
	for {
		message, err := signal.subscribers["raw"].Wait(signal.ctx)

		if err != nil {
		return err
		}

		if message == nil || message.Value == nil {
		continue
		}

		sm, ok := signalpool.SocketMessageFromValue(message.Value)

		if !ok {
		continue
		}

		switch sm.Channel {
		case public.TradesChannel:
			trades := signalpool.GetTrades(sm)

			if err := signal.observeTrades(trades); err != nil {
				errnie.Error(err, "hawkes: observe trades")
			}
		case public.TickerChannel:
			tickers := signalpool.GetTickers(sm)

			if err := signal.observeTickers(tickers); err != nil {
				errnie.Error(err, "hawkes: observe tickers")
			}
		}
	}
}

func (signal *Signal) observeTickers(tickers []market.TickerUpdate) error {
	var err error

	for _, ticker := range tickers {
		if ticker.Last <= 0 {
			continue
		}

		raw, ok := signal.symbols.Load(ticker.Symbol)

		if !ok {
			continue
		}

		at, parseErr := hawkesTickerTime(ticker)

		if parseErr != nil {
			err = errors.Join(err, parseErr)
			continue
		}

		err = errors.Join(
			err,
			signal.publishMeasurement(ticker.Symbol, raw.(*symbolState), ticker.Last, at),
		)
	}

	return err
}

func (signal *Signal) observeTrades(trades []market.TradeUpdate) error {
	touches := signal.tradeScratch[:0]
	indexBySymbol := make(map[string]int, len(trades))

	for _, trade := range trades {
		if trade.Price <= 0 || trade.Qty <= 0 {
			continue
		}

		stored, _ := signal.symbols.LoadOrStore(trade.Symbol, newSymbolState(signal.categories, signal.classifier))
		state := stored.(*symbolState)
		state.append(trade)

		if touchIndex, ok := indexBySymbol[trade.Symbol]; ok {
			touches[touchIndex].last = trade.Price
			continue
		}

		indexBySymbol[trade.Symbol] = len(touches)
		touches = append(touches, tradeTouch{
			symbol: trade.Symbol,
			state:  state,
			last:   trade.Price,
		})
	}

	signal.tradeScratch = touches

	if len(touches) == 0 {
		return nil
	}

	return signal.publishTouches(touches)
}

func (signal *Signal) publishTouches(touches []tradeTouch) error {
	tasks := make([]*qpool.ResultWait[any], 0, len(touches))

	for _, touch := range touches {
		tasks = append(tasks, signal.pool.Schedule(fmt.Sprintf("%s:parallel:%p", "hawkes", &tasks), func(ctx context.Context) (any, error) {
			now := touchLastTime(touch.state)
			return nil, signal.publishMeasurement(touch.symbol, touch.state, touch.last, now)
		}))
	}

	var err error

	for _, task := range tasks {
		value, getErr := task.Get(signal.ctx)
		if getErr != nil {
			err = errors.Join(err, getErr)
			continue
		}
		err = errors.Join(err, value.Error)
	}

	return err
}

func (signal *Signal) publishMeasurement(
	symbol string,
	state *symbolState,
	last float64,
	at time.Time,
) error {
	measurement, standout, err := state.measure(at)

	if err != nil {
		return err
	}

	if measurement.Source == types.SourceNone {
		return nil
	}

	measurement.Symbol = symbol
	measurement.Last = last

	signal.calibrator.Observe(measurement.Strength, signal.classifier)

	telemetry := signal.calibrator.Snapshot(signal.classifier)
	telemetry.Observation = measurement.Strength
	if err := types.AssignCategorySurpriseSNR(
		&measurement, signal.surpriseField, measurement.Category,
	); err != nil {
		return err
	}

	if err := signal.rawDump.Write(rawRecord{
		TimestampUnixNano: at.UnixNano(),
		Symbol:            measurement.Symbol,
		Category:          measurement.Category,
		Strength:          measurement.Strength,
		Confidence:        measurement.Confidence,
		SNR:               measurement.SNR,
		Standout:          standout,
		Last:              measurement.Last,
		SpreadBPS:         measurement.SpreadBPS,
	}); err != nil {
		return err
	}

	if err := measurement.Send(signal.pool); err != nil {
		return err
	}

	if ui := signal.broadcasts["ui"]; ui != nil {
		ui.Send(&qpool.QValue[any]{
			Value: numeric.GaugePayload(
				measurement.Source.String(),
				measurement.Symbol,
				measurement.Category,
				measurement,
				telemetry,
			),
		})
	}

	return nil
}

func hawkesTickerTime(ticker market.TickerUpdate) (time.Time, error) {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.000000Z"} {
		if at, err := time.Parse(layout, ticker.Timestamp); err == nil {
			return at, nil
		}
	}

	return time.Time{}, fmt.Errorf("hawkes: ticker timestamp is required for %s", ticker.Symbol)
}

func touchLastTime(state *symbolState) time.Time {
	state.mu.Lock()
	defer state.mu.Unlock()

	if tickCount := len(state.ticks); tickCount > 0 {
		return state.ticks[tickCount-1].Timestamp
	}

	return time.Now()
}

func (signal *Signal) Close() error {
	signal.cancel()
	return signal.rawDump.Close()
}
