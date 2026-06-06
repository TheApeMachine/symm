package liquidity

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/bus"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/market/perspectives/types"
	"github.com/theapemachine/symm/numeric"
	"github.com/theapemachine/symm/numeric/adaptive"
	"github.com/theapemachine/symm/rawdump"
	signalpool "github.com/theapemachine/symm/signal"
)

const (
	minLiquidityPeers = 2
	rawSubscriberID   = "signal/liquidity:raw"
)

var liquidityDefaultBandEdges = []float64{0.75, 1.25}

/*
Signal ranks a symbol's quote volume against the live cross-section of its peers
and maps the standing onto the scarcity perspective. It is a cross-asset signal:
the verdict for one symbol depends on where its quote volume sits in the peer
median. Confidence is classification clarity — margin to the nearest peer
quartile; SNR scores category standout — peer deviation from the median — against
the symbol's own recent baseline.

| Category          | Quote Volume vs peer median | Market "Feel"     |
|:------------------|:----------------------------|:------------------|
| Robust Liquidity  | well above (>= 1.25x)       | Deep / easy fills |
| Median Depth      | around the median           | Normal            |
| Extreme Scarcity  | well below (< 0.75x)        | Thin / fragile    |
*/
type Signal struct {
	ctx           context.Context
	cancel        context.CancelFunc
	pool          *qpool.Q
	broadcasts    map[string]*qpool.BroadcastGroup
	subscribers   map[string]*qpool.Subscriber
	symbols       sync.Map // symbol -> float64 (daily quote volume)
	tracked       sync.Map // symbol -> *types.Category
	surpriseField *types.CategorySurpriseField
	classifier    *adaptive.Classifier
	calibrator    *numeric.BandCalibrator
	rawDump       *rawdump.Writer
}

func NewSignal(ctx context.Context, pool *qpool.Q) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	pooledCalibrator := numeric.NewSignalCalibrator(
		liquidityDefaultBandEdges,
		[]float64{0, 1, 2},
		[]string{"extreme_scarcity", "median_depth", "robust_liquidity"},
		[]float64{0.20, 0.50, 0.30},
		numeric.DefaultCalibratorConfig("strength"),
		"liquidity",
	)

	surpriseField, err := types.NewCategorySurpriseField([]types.CategoryType{
		types.CategoryExtremeScarcity,
		types.CategoryMedianDepth,
		types.CategoryRobustLiquidity,
	}, types.DefaultCategorySurpriseAlpha)

	if err != nil {
		cancel()
		errnie.Error(err, "signal/liquidity")
		return nil
	}

	signal := &Signal{
		ctx:           ctx,
		cancel:        cancel,
		pool:          pool,
		broadcasts:    make(map[string]*qpool.BroadcastGroup),
		subscribers:   make(map[string]*qpool.Subscriber),
		surpriseField: surpriseField,
		classifier:    pooledCalibrator.Classifier,
		calibrator:    pooledCalibrator.Calibrator,
		rawDump:       rawdump.Open("liquidity"),
	}

	for _, channel := range []string{"raw"} {
		signal.broadcasts[channel] = bus.Group(pool, channel, 10*time.Millisecond)
		signal.subscribers[channel] = signal.broadcasts[channel].Subscribe(rawSubscriberID, 1024)
	}

	signal.broadcasts["measurements"] = bus.Group(pool, "measurements", 10*time.Millisecond)
	signal.broadcasts["ui"] = bus.Group(pool, "ui", 10*time.Millisecond)

	errnie.Info("signal/liquidity ready", "signal/liquidity")

	return signal
}

func (signal *Signal) Tick() error {
	for {
		select {
		case <-signal.ctx.Done():
			return signal.ctx.Err()
		case message := <-signal.subscribers["raw"].Incoming:
			if message == nil || message.Value == nil {
				continue
			}

			sm, ok := signalpool.SocketMessageFromValue(message.Value)

			if !ok {
				continue
			}

			switch sm.Channel {
			case public.TickerChannel:
				tickers := signalpool.GetTickers(sm)

				if err := signal.publishTickers(tickers); err != nil {
					errnie.Error(err, "liquidity: publish tickers")
				}
			}
		}
	}
}

/*
measure records the latest quote volume for the ticking symbol and ranks it
against the live peer cross-section.
*/
func (signal *Signal) publishTickers(tickers []market.TickerUpdate) error {
	rows := make([]market.TickerUpdate, 0, len(tickers))

	for _, ticker := range tickers {
		if ticker.Last <= 0 {
			continue
		}

		signal.symbols.Store(ticker.Symbol, ticker.Volume*ticker.Last)
		rows = append(rows, ticker)
	}

	if len(rows) == 0 {
		return nil
	}

	volumes := signal.volumeSnapshot()
	tasks := make([]chan *qpool.QValue[any], 0, len(rows))

	for _, row := range rows {
		tasks = append(tasks, signal.pool.ScheduleFast(signal.ctx, func(context.Context) (any, error) {
			measurement, standout, err := signal.measureFromVolumes(row, volumes)

			if err != nil {
				return nil, err
			}

			if measurement.Symbol == "" {
				return nil, nil
			}

			telemetry, _ := numeric.ObserveGaugeTelemetry(
				signal.calibrator,
				signal.classifier,
				measurement.Strength,
				standout,
			)

			if err := types.AssignCategorySurpriseSNR(
				&measurement, signal.surpriseField, measurement.Category,
			); err != nil {
				return nil, err
			}

			if err := signal.rawDump.Write(rawRecord{
				TimestampUnixNano: time.Now().UTC().UnixNano(),
				Symbol:            measurement.Symbol,
				Category:          measurement.Category,
				Strength:          measurement.Strength,
				Confidence:        measurement.Confidence,
				SNR:               measurement.SNR,
				Standout:          standout,
				Last:              measurement.Last,
				SpreadBPS:         measurement.SpreadBPS,
			}); err != nil {
				return nil, err
			}

			if err := measurement.Send(signal.pool); err != nil {
				return nil, err
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

			return nil, nil
		}))
	}

	var err error

	for _, task := range tasks {
		value := <-task
		err = errors.Join(err, value.Error)
	}

	return err
}

func (signal *Signal) measure(row market.TickerUpdate) (types.Measurement, float64, error) {
	signal.symbols.Store(row.Symbol, row.Volume*row.Last)

	volumes := signal.volumeSnapshot()

	return signal.measureFromVolumes(row, volumes)
}

func (signal *Signal) measureFromVolumes(
	row market.TickerUpdate,
	volumes map[string]float64,
) (types.Measurement, float64, error) {
	quoteVol := volumes[row.Symbol]

	peers := make([]float64, 0, len(volumes)-1)

	for symbol, volume := range volumes {
		if symbol == row.Symbol || volume <= 0 {
			continue
		}

		peers = append(peers, volume)
	}

	if quoteVol <= 0 || len(peers) < minLiquidityPeers {
		return types.Measurement{}, 0, nil
	}

	median := numeric.PercentileSorted(numeric.CopySorted(peers), 0.5)

	if median <= 0 {
		return types.Measurement{}, 0, fmt.Errorf(
			"liquidity: non-positive peer median for %s",
			row.Symbol,
		)
	}

	ratio := quoteVol / median
	raw := signal.strength(ratio)
	category, confidence, standout, err := liquidityReading(quoteVol, peers)

	if err != nil {
		return types.Measurement{}, 0, err
	}

	trackedRaw, _ := signal.tracked.LoadOrStore(
		row.Symbol,
		types.NewCategory(types.CategoryTypeNone),
	)
	tracked := trackedRaw.(*types.Category)

	if err := tracked.Observe(category, confidence); err != nil {
		return types.Measurement{}, 0, err
	}

	return types.Measurement{
		Symbol:     row.Symbol,
		Source:     types.SourceLiquidity,
		Category:   category,
		Last:       row.Last,
		Volume:     quoteVol,
		Strength:   raw,
		Confidence: confidence,
	}, standout, nil
}

func (signal *Signal) volumeSnapshot() map[string]float64 {
	volumes := make(map[string]float64)

	signal.symbols.Range(func(key, value any) bool {
		volumes[key.(string)] = value.(float64)

		return true
	})

	return volumes
}

/*
strength is the raw distance of quote volume from the peer median, in either
direction, for dashboard gauges only.
*/
func (signal *Signal) strength(ratio float64) float64 {
	if ratio < 1 {
		return 1 / ratio
	}

	return ratio
}

func (signal *Signal) Close() error {
	signal.cancel()
	return signal.rawDump.Close()
}
