package pumpdump

import (
	"context"
	"sync"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

/*
Signal owns pump-cycle measurements derived from executed trade lift and the
reconstructed book's midpoint and spread. It reads each market
fact from its authoritative stream without treating them as independent
corroborating signals.
*/
type Signal struct {
	status        types.Status
	ctx           context.Context
	cancel        context.CancelFunc
	api           *websocket.API
	algo          *equation.Ignition
	lastTrades    map[string]time.Time
	volumes       map[string]float64
	planner       *strategy.Planner
	ui            chan []byte
	subscriptions map[string]*types.Subscription[any]
	subscribers   *sync.Map
	subscribeMu   sync.Mutex
}

/*
NewSignal creates an empty per-symbol pump state whose baseline capacity is the
same explicit retention bound used by the production market feed.
*/
func NewSignal(
	ctx context.Context,
	api *websocket.API,
	planner *strategy.Planner,
	ui chan []byte,
	subscriptions map[string]*types.Subscription[any],
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		status:        types.INITIALIZING,
		ctx:           ctx,
		cancel:        cancel,
		api:           api,
		algo:          equation.NewIgnition(viper.GetViper().GetInt("signals.pumpdump.baselineCapacity")),
		lastTrades:    make(map[string]time.Time),
		volumes:       make(map[string]float64),
		planner:       planner,
		ui:            ui,
		subscriptions: subscriptions,
		subscribers:   &sync.Map{},
	}

	signal.status = types.READY
	signal.run()
	return signal
}

/*
Name returns the signal source identity.
*/
func (signal *Signal) Name() string {
	return string(types.SourcePumpDump)
}

func (signal *Signal) Status() types.Status {
	return signal.status
}

func (signal *Signal) Subscribe(
	channel string,
	subscription *types.Subscription[any],
) *types.Subscription[any] {
	return utils.Subscribe(
		signal.subscribers,
		channel,
		subscription,
	)
}

func (signal *Signal) ensureState() {
	if signal.lastTrades == nil {
		signal.lastTrades = make(map[string]time.Time)
	}

	if signal.volumes == nil {
		signal.volumes = make(map[string]float64)
	}
}

func (signal *Signal) run() {
	go func() {
		for {
			select {
			case <-signal.ctx.Done():
				return
			case message := <-signal.subscriptions["thesis"].Channel:
				if thesis, ok := message.(*types.Thesis); ok {
					thesis.AppendMeasurements(
						types.SourcePumpDump,
						signal.Measure(thesis),
						types.Stamp{
							At:     time.Now(),
							Entity: types.MarketTicker,
							Source: types.SourcePumpDump,
						},
					)

					utils.Fanout(signal.subscribers, signal.Name(), thesis)
				}
			}
		}
	}()
}

/*
Measure produces the Measurements for the pumpdump signal.
*/
func (signal *Signal) Measure(
	thesis *types.Thesis,
) []*types.Measurement {
	measurements := make([]*types.Measurement, 0)
	_, trades, _ := thesis.Market()
	signal.ensureState()

	if len(trades) == 0 {
		return nil
	}

	var lastTime time.Time

	for _, trade := range trades {
		if trade.Qty <= 0 || trade.Price.Sign() <= 0 {
			continue
		}

		previous, ok := signal.lastTrades[trade.Symbol]

		if ok && !trade.Timestamp.After(previous) {
			continue
		}

		value, ok := thesis.Tickers.Load(trade.Symbol)

		if !ok {
			continue
		}

		ticker, ok := value.(kraken.TickerData)

		if !ok || ticker.Bid == nil || ticker.Ask == nil ||
			ticker.Bid.Sign() <= 0 || ticker.Ask.Sign() <= 0 ||
			ticker.Ask.Cmp(ticker.Bid) <= 0 {
			continue
		}

		previous, ok = signal.lastTrades[trade.Symbol]

		if ok && !trade.Timestamp.After(previous) {
			continue
		}

		cumulativeVolume := signal.volumes[trade.Symbol] + trade.Qty
		signal.lastTrades[trade.Symbol] = trade.Timestamp
		signal.volumes[trade.Symbol] = cumulativeVolume

		if lastTime.IsZero() || trade.Timestamp.After(lastTime) {
			lastTime = trade.Timestamp
		}

		mid := (ticker.Bid.Float64() + ticker.Ask.Float64()) / 2

		if mid <= 0 {
			continue
		}

		output, _, maturity, err := signal.algo.Measure(equation.IgnitionInput{
			At:     trade.Timestamp,
			Symbol: trade.Symbol,
			Last:   trade.Price.Float64(),
			Volume: cumulativeVolume,
			Ask:    ticker.Ask.Float64(),
			Bid:    ticker.Bid.Float64(),
		})

		if err != nil {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"pumpdump: failed to measure ignition",
				err,
			))

			continue
		}

		measurements = append(measurements, &types.Measurement{
			Source:   types.SourcePumpDump,
			Symbol:   trade.Symbol,
			At:       trade.Timestamp,
			Maturity: maturity,
			Validity: types.ObservationValidity(1),
			Scale: types.ScaleReference{
				Kind:    types.ScaleObservationWindow,
				From:    lastTime,
				Through: trade.Timestamp,
			},
			Metrics: map[string]types.MetricSample{
				types.MetricKey(types.MetricRVOL, types.SideNone): {
					Raw:        output.RVOL,
					Normalized: types.NormalizeFinite(output.RVOL),
					Unit:       types.UnitDimensionless,
				},
				types.MetricKey(types.MetricPrecursor, types.SideNone): {
					Raw:        output.Precursor,
					Normalized: types.NormalizeFinite(output.Precursor),
					Unit:       types.UnitDimensionless,
				},
				types.MetricKey(types.MetricSpread, types.SideNone): {
					Raw:        output.Spread,
					Normalized: types.NormalizeRatio(output.Spread, mid),
					Unit:       types.UnitQuoteCurrency,
				},
				types.MetricKey(types.MetricCompression, types.SideNone): {
					Raw:        output.Compression,
					Normalized: types.NormalizeFinite(output.Compression),
					Unit:       types.UnitDimensionless,
				},
				types.MetricKey(types.MetricIgnition, types.SideNone): {
					Raw:        output.Ignition,
					Normalized: types.NormalizeFinite(output.Ignition),
					Unit:       types.UnitDimensionless,
				},
				types.MetricKey(types.MetricTrend, types.SideNone): {
					Raw:        output.Trend,
					Normalized: types.NormalizeFinite(output.Trend),
					Unit:       types.UnitDimensionless,
				},
				types.MetricKey(types.MetricExhaustion, types.SideNone): {
					Raw:        output.Exhaustion,
					Normalized: types.NormalizeFinite(output.Exhaustion),
					Unit:       types.UnitDimensionless,
				},
				types.MetricKey(types.MetricStrength, types.SideNone): {
					Raw:        output.Strength,
					Normalized: types.NormalizeFinite(output.Strength),
					Unit:       types.UnitDimensionless,
				},
			},
		})
	}

	focusMeasurements := make([]*types.Measurement, 0)

	for _, measurement := range measurements {
		if measurement.Symbol == types.Focus() {
			focusMeasurements = append(focusMeasurements, measurement)
		}
	}

	if len(focusMeasurements) > 0 {
		utils.Publish(signal.ui, datura.NewMap("measurements", focusMeasurements))
	}

	return measurements
}

/*
Close releases the receiver's owned resources so shutdown does not leave
active market-data producers.
*/
func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	return nil
}
