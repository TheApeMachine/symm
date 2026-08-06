package correlation

import (
	"context"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

/*
Signal measures whether a symbol is moving with the cohort, against it, beyond
it, or without a stable relation to it. Categories belong in logic; this signal
emits numerical scores only.
*/
type Signal struct {
	status        types.Status
	ctx           context.Context
	cancel        context.CancelFunc
	api           *websocket.API
	planner       *strategy.Planner
	section       *Section
	ui            chan []byte
	subscriptions map[string]*types.Subscription[any]
	subscribers   *sync.Map
	mu            sync.Mutex
}

/*
NewSignal creates correlation measurement state for central market cuts so
successive ticks can establish real price relationships.
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
		planner:       planner,
		section:       NewSection(),
		ui:            ui,
		subscriptions: subscriptions,
		subscribers:   &sync.Map{},
	}

	signal.run()
	signal.status = types.READY

	return signal
}

/*
Name returns the signal source identity.
*/
func (signal *Signal) Name() string {
	return string(types.SourceCorrelation)
}

func (signal *Signal) Status() types.Status {
	return signal.status
}

func (signal *Signal) Subscribe(
	channel string, subscription *types.Subscription[any],
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
					measurements := signal.Measure(thesis)

					if len(measurements) > 0 {
						thesis.AppendMeasurements(measurements, types.MeasurementsReady(measurements))
						utils.Fanout(signal.subscribers, signal.Name(), thesis)
					}

				}
			}
		}
	}()
}

func (signal *Signal) Measure(thesis *types.Thesis) []*types.Measurement {
	tickers := thesis.MarketTickers()

	if len(tickers) == 0 {
		return nil
	}

	scoresBySymbol, err := signal.section.Measure(tickers)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent, "correlation: failed to measure tickers", err,
		))

		return nil
	}

	if len(scoresBySymbol) == 0 {
		return nil
	}

	latestAtBySymbol := make(map[string]time.Time, len(tickers))

	for _, row := range tickers {
		symbol := strings.TrimSpace(row.Symbol)

		if symbol == "" || row.Timestamp.IsZero() {
			continue
		}

		if !row.Timestamp.After(latestAtBySymbol[symbol]) {
			continue
		}

		latestAtBySymbol[symbol] = row.Timestamp
	}

	measurements := make([]*types.Measurement, 0)
	out := make([]*types.Measurement, 0)

	validity := types.MeasurementValidity{
		State:     types.ValidityValid,
		Readiness: types.ReadinessObservation,
	}

	for symbol, scores := range scoresBySymbol {
		at := latestAtBySymbol[symbol]

		if at.IsZero() {
			at = signal.section.LastAt(symbol)
		}

		if at.IsZero() {
			continue
		}

		metrics, normalizationReady := correlationMetrics(scores)
		measurementValidity := validity

		if !normalizationReady {
			measurementValidity.State = types.ValidityInvalid
			measurementValidity.Reason = "correlation normalization contract violated"
		}

		measurement := &types.Measurement{
			Source:   types.SourceCorrelation,
			Symbol:   symbol,
			At:       at,
			Validity: measurementValidity,
			Metrics:  metrics,
		}

		measurements = append(measurements, measurement)

		if symbol == types.Focus() {
			out = append(out, measurement)
		}
	}

	if len(out) > 0 {
		utils.Publish(signal.ui, datura.NewMap(
			"measurements", out,
		))
	}

	return measurements
}

/*
correlationMetrics validates the equation-defined normalized domains. Pair
correlation and category scores are bounded, signed correlation retains its
direction, and relative energy is already a symbol/peer-energy ratio.
*/
func correlationMetrics(
	scores map[string]float64,
) (map[string]types.MetricSample, bool) {
	type reading struct {
		name   string
		metric types.MetricType
	}

	readings := []reading{
		{"correlation", types.MetricCorrelation},
		{"signed", types.MetricSigned},
		{"relativeEnergy", types.MetricRelativeEnergy},
		{"herdScore", types.MetricHerdScore},
		{"alphaScore", types.MetricAlphaScore},
		{"noiseScore", types.MetricNoiseScore},
		{"stressScore", types.MetricStressScore},
		{"peakScore", types.MetricPeakScore},
		{"strength", types.MetricStrength},
	}
	metrics := make(map[string]types.MetricSample, len(readings))
	valid := true

	for _, item := range readings {
		raw, exists := scores[item.name]
		normalized := normalizedCorrelationMetric(item.metric, raw)

		if !exists || normalized == nil {
			valid = false
		}

		metrics[types.MetricKey(item.metric, types.SideNone)] = types.MetricSample{
			Raw:        raw,
			Normalized: normalized,
			Unit:       types.UnitDimensionless,
		}
	}

	return metrics, valid
}

func normalizedCorrelationMetric(metric types.MetricType, raw float64) *float64 {
	if math.IsNaN(raw) || math.IsInf(raw, 0) {
		return nil
	}

	if metric == types.MetricSigned {
		if raw < -1 || raw > 1 {
			return nil
		}

		value := raw

		return &value
	}

	if metric == types.MetricRelativeEnergy {
		if raw <= 0 {
			return nil
		}

		value := raw

		return &value
	}

	switch metric {
	case types.MetricCorrelation,
		types.MetricHerdScore,
		types.MetricAlphaScore,
		types.MetricNoiseScore,
		types.MetricStressScore,
		types.MetricPeakScore,
		types.MetricStrength:
		if raw < 0 || raw > 1 {
			return nil
		}
	default:
		return nil
	}

	value := raw

	return &value
}

/*
Close releases the receiver's owned resources so shutdown does not leave
active market-data producers.
*/
func (signal *Signal) Close() error {
	signal.cancel()
	return nil
}
