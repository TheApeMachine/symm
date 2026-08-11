package correlation

import (
	"context"
	"sort"
	"strings"
	"sync/atomic"

	"github.com/google/uuid"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
	"golang.org/x/sync/errgroup"
)

/*
Signal measures whether a symbol is moving with the cohort, against it, beyond
it, or without a stable relation to it. Categories belong in logic; this signal
emits numerical scores only.
*/
type Signal struct {
	status    atomic.Value
	ctx       context.Context
	cancel    context.CancelFunc
	api       *websocket.API
	section   *Section
	ui        chan []byte
	thesis    *types.Thesis
	semaphore chan struct{}
}

/*
NewSignal creates correlation measurement state for central market cuts so
successive ticks can establish real price relationships.
*/
func NewSignal(
	ctx context.Context,
	api *websocket.API,
	ui chan []byte,
	thesis *types.Thesis,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:       ctx,
		cancel:    cancel,
		api:       api,
		section:   NewSection(),
		ui:        ui,
		thesis:    thesis,
		semaphore: make(chan struct{}, 1),
	}

	signal.status.Store(types.INITIALIZING)
	signal.thesis.Subscribe(types.SourceCorrelation, signal.semaphore, &signal.status)
	signal.status.Store(types.READY)
	signal.run()

	return signal
}

/*
Name returns the signal source identity.
*/
func (signal *Signal) Name() string {
	return string(types.SourceCorrelation)
}

func (signal *Signal) Status() types.Status {
	return signal.status.Load().(types.Status)
}

func (signal *Signal) run() {
	go func() {
		for {
			select {
			case <-signal.ctx.Done():
				return
			case <-signal.semaphore:
				measurements := signal.Measure(signal.thesis)

				if len(measurements) > 0 {
					signal.thesis.AppendMeasurements(
						types.SourceCorrelation, measurements, true,
					)
				}

				signal.status.Store(types.READY)
			}
		}
	}()
}

func (signal *Signal) Measure(thesis *types.Thesis) []*types.Measurement {
	tickers := thesis.MarketTickers(types.SourceCorrelation)

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

	latestBySymbol := make(map[string]kraken.TickerData, len(tickers))

	for _, row := range tickers {
		symbol := strings.TrimSpace(row.Symbol)

		if symbol == "" || row.Timestamp.IsZero() {
			continue
		}

		if previous, found := latestBySymbol[symbol]; found &&
			!row.Timestamp.After(previous.Timestamp) {
			continue
		}

		latestBySymbol[symbol] = row
	}

	symbols := make([]string, 0, len(scoresBySymbol))

	for symbol := range scoresBySymbol {
		symbols = append(symbols, symbol)
	}
	sort.Strings(symbols)
	measurements := make([]*types.Measurement, len(symbols))
	out := make([]*types.Measurement, 0)

	group, _ := errgroup.WithContext(signal.ctx)

	for index, symbol := range symbols {
		measurementIndex := index
		scores := scoresBySymbol[symbol]

		group.Go(func() error {
			row, updated := latestBySymbol[symbol]

			if !updated || row.Timestamp.IsZero() || row.Last == nil {
				return nil
			}

			metrics, valid := correlationMetrics(scores)

			if !valid {
				return nil
			}

			measurement := &types.Measurement{
				ID:      uuid.NewString(),
				Source:  types.SourceCorrelation,
				Symbol:  symbol,
				At:      row.Timestamp,
				Metrics: metrics,
			}
			measurement.PutMetric(
				types.MetricLastPrice,
				types.SideNone,
				types.MetricSample{
					Raw:  row.Last.Float64(),
					Unit: types.UnitQuoteCurrency,
				},
			)

			measurements[measurementIndex] = measurement

			return nil
		})
	}

	if err := group.Wait(); err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"correlation: parallel measurement construction failed",
			err,
		))
		return nil
	}

	compacted := measurements[:0]

	for _, measurement := range measurements {
		if measurement == nil {
			continue
		}

		compacted = append(compacted, measurement)

		if measurement.Symbol == types.Focus() {
			out = append(out, measurement)
		}
	}
	measurements = compacted

	if len(out) > 0 {
		utils.Publish(signal.ui, datura.NewMap(
			"measurements", out,
		))
	}

	return measurements
}

/*
correlationMetrics maps the complete equation output onto measurement keys.
The equation already returns dimensionless scores and ratios, so Normalized
retains those values without applying a second transformation.
*/
func correlationMetrics(
	scores map[string]float64,
) (map[string]types.MetricSample, bool) {
	type reading struct {
		name   string
		metric types.MetricType
	}

	readings := []reading{
		{"snr", types.MetricSNR},
		{"correlation", types.MetricCorrelation},
		{"signed", types.MetricSigned},
		{"relativeEnergy", types.MetricRelativeEnergy},
		{"herdScore", types.MetricHerdScore},
		{"alphaScore", types.MetricAlphaScore},
		{"noiseScore", types.MetricNoiseScore},
		{"stressScore", types.MetricStressScore},
	}

	metrics := make(map[string]types.MetricSample, len(readings))
	valid := true

	for _, item := range readings {
		raw, exists := scores[item.name]
		var normalized *float64

		if !exists {
			valid = false
		} else {
			normalized = &raw
		}

		metrics[types.MetricKey(item.metric, types.SideNone)] = types.MetricSample{
			Raw:        raw,
			Normalized: normalized,
			Unit:       types.UnitDimensionless,
		}
	}

	return metrics, valid
}

/*
Close releases the receiver's owned resources so shutdown does not leave
active market-data producers.
*/
func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	if signal.section != nil {
		signal.section.Close()
	}

	return nil
}
