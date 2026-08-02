package pumpdump

import (
	"context"
	"sync"
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/equation"
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
	out := make([]*types.Measurement, 0)

	_, trades, books := thesis.Market()

	if len(trades) == 0 {
		return measurements
	}

	for _, trade := range trades {
		if trade.Qty <= 0 || trade.Price.Sign() <= 0 {
			continue
		}

		found, ok := books.Load(trade.Symbol)

		if !ok || found == nil {
			continue
		}

		book, ok := found.(*spotbook.Book)

		if !ok || book == nil {
			continue
		}

		mid := book.Midpoint().Float64()

		if mid <= 0 {
			continue
		}

		/*
			A one-sided book has no best bid or ask to quote against, which is
			exactly what a violent pump produces when one side is swept. The
			midpoint alone does not imply both sides carry levels.

			Each touch is read once into a local, because the websocket reader
			replaces these levels as the book updates and a pointer that
			survives the nil check can still be gone by the time it is
			dereferenced.
		*/
		ask, bid := book.Asks.High, book.Bids.Low

		if ask == nil || bid == nil || ask.Price == nil || bid.Price == nil {
			continue
		}

		output, ready, maturity, err := signal.algo.Measure(equation.IgnitionInput{
			At:     trade.Timestamp,
			Symbol: trade.Symbol,
			Last:   trade.Price.Float64(),
			Volume: trade.Qty,
			Ask:    ask.Price.Float64(),
			Bid:    bid.Price.Float64(),
		})

		if err != nil {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"pumpdump: failed to measure ignition",
				err,
			))

			continue
		}

		validity := types.MeasurementValidity{
			State:     types.ValidityValid,
			Readiness: types.ReadinessObservation,
		}

		if !ready {
			validity.State = types.ValidityProvisional
			validity.Reason = "ignition baseline not ready"
		}

		measurement := &types.Measurement{
			Source:   types.SourcePumpDump,
			Symbol:   trade.Symbol,
			At:       trade.Timestamp,
			Maturity: maturity,
			Validity: validity,
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
		}

		measurements = append(measurements, measurement)

		if measurement.Symbol == types.Focus() {
			out = append(out, measurement)
		}
	}

	if len(out) > 0 {
		utils.Publish(signal.ui, datura.NewMap("measurements", out))
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
